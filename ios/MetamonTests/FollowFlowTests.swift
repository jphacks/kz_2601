import XCTest
import MetamonDSL
import MetamonServices
import MetamonRenderer
@testable import Metamon

/// フォローの一覧を返す API(#270)。付け外しを記録し、失敗を切り替えられる。
private final class FakeFollows: FollowsClient, @unchecked Sendable {
    private let lock = NSLock()
    var error: Error?
    private(set) var calls: [String] = []

    func list(appId: String) async throws -> [String] {
        lock.withLock { calls.append("list") }
        if let error { throw error }
        return ["u-alice"]
    }
    func setFollow(appId: String, uid: String, on: Bool) async throws -> Bool {
        lock.withLock { calls.append("\(on ? "follow" : "unfollow") \(uid)") }
        if let error { throw error }
        return on
    }
}

/// #270: フォローの一覧の読み込み・ボタンの出し分け・失敗の文言。
@MainActor
final class FollowFlowTests: XCTestCase {
    override func setUp() {
        super.setUp()
        FollowsUI.shared.reset()
    }

    private func runtime(_ fake: FakeFollows?) async throws -> AppRuntime {
        let dsl = try DSLExamples.load("timeline")
        var services = BuildConfig.make(environment: .mock)
        services.follows = fake
        let rt = AppRuntime(document: AppDocument(appId: "tl-\(UUID().uuidString.prefix(6))", version: 1, dsl: dsl), services: services)
        _ = await rt.createRecord(entity: "posts", data: ["body": "はじめまして"])
        return rt
    }

    func testMockHasNoFollows() async throws {
        let rt = try await runtime(nil)
        XCTAssertNil(rt.followStore)
        XCTAssertNil(rt.followedAuthors(), "フォローの無い版(モック)ではフォロー中の一覧は空")
        await rt.runFollow(Action(type: .follow, entity: "posts"), ActionContext(route: Route(screen: "post_form")))
        XCTAssertEqual(rt.toast, L10n.tr("このアプリでは今はフォローを使えません"))
    }

    func testLoadsFollowsAndSkipsOwnPost() async throws {
        let fake = FakeFollows()
        let rt = try await runtime(fake)
        let store = try XCTUnwrap(rt.followStore)
        try await store.load()
        XCTAssertEqual(rt.followedAuthors(), ["u-alice"])
        let mine = try XCTUnwrap(rt.records(for: "posts").first)
        XCTAssertNil(rt.followState(entity: "posts", recordId: mine.id), "自分の投稿にはボタンを出さない")
        await rt.runFollow(Action(type: .follow, entity: "posts"), ActionContext(route: Route(screen: "post_form"), recordId: mine.id))
        XCTAssertEqual(rt.toast, L10n.tr("ほかの人の投稿を開いてからフォローしてください"))
        XCTAssertEqual(fake.calls, ["list"], "自分はフォローしない")
    }

    func testErrorText() {
        let full = APIError.server(status: 403, code: "forbidden", message: "", details: ["reason": .string(FollowsReason.full)])
        XCTAssertEqual(AppRuntime.followErrorText(full), L10n.tr("フォローできる人数(500 人)を超えています"))
        let gone = APIError.server(status: 404, code: "not_found", message: "", details: ["reason": .string(FollowsReason.notInApp)])
        XCTAssertEqual(AppRuntime.followErrorText(gone), L10n.tr("この人はもうこのアプリに参加していません"))
        XCTAssertEqual(AppRuntime.followErrorText(URLError(.notConnectedToInternet)), L10n.tr("オフラインのあいだはフォローできません"))
    }
}
