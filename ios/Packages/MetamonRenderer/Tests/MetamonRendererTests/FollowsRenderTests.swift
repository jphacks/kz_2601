import XCTest
import SwiftUI
import Observation
import MetamonDSL
@testable import MetamonRenderer

/// テスト用のランタイム(投稿・自分の uid・フォローしている人)。
@MainActor
@Observable
private final class TimelineRuntime: DSLRuntime, FollowsHost {
    var dsl: AppDSL
    var paths: [String: [Route]] = [:]
    var selectedTab = ""
    var loading: Set<String> = []
    var posts: [Record]
    var currentUserId: String? = "me"
    var followed: Set<String>?

    init(dsl: AppDSL, posts: [Record], followed: Set<String>?) {
        self.dsl = dsl
        self.posts = posts
        self.followed = followed
    }

    func records(for entity: String) -> [Record] { entity == "posts" ? posts : [] }
    func load(entities: Set<String>, force: Bool) async {}
    func run(_ action: Action, _ ctx: ActionContext) async {}
    func close(_ route: Route) {}
    func pathBinding(_ stack: String) -> Binding<[Route]> { .constant([]) }

    func followedAuthors() -> Set<String>? { followed }
    func followState(entity: String, recordId: String) -> Bool? {
        guard let followed, let author = posts.first(where: { $0.id == recordId })?.author, author != currentUserId else { return nil }
        return followed.contains(author)
    }
}

/// #270: フォローした人の投稿だけの一覧と、フォロー / 解除のボタンの出し分け。
@MainActor
final class FollowsRenderTests: XCTestCase {
    private func timeline() throws -> AppDSL {
        var url = URL(fileURLWithPath: #filePath)
        for _ in 0..<6 { url.deleteLastPathComponent() }
        return try AppDSL.load(contentsOf: url.appending(path: "docs/dsl/examples/timeline.json"))
    }

    private let posts = [
        Record(id: "1", data: ["body": "a"], author: "alice"),
        Record(id: "2", data: ["body": "b"], author: "bob"),
        Record(id: "3", data: ["body": "c"], author: "me"),
    ]

    func testFollowingListShowsFollowedAuthorsOnly() throws {
        let dsl = try timeline()
        let following = try XCTUnwrap(dsl.screen("following")?.allComponents.first { $0.type == .list })
        let everyone = try XCTUnwrap(dsl.screen("everyone")?.allComponents.first { $0.type == .list })
        let rt = TimelineRuntime(dsl: dsl, posts: posts, followed: ["alice"])
        XCTAssertEqual(rt.filteredRecords(following).map(\.id), ["1"])
        XCTAssertEqual(rt.filteredRecords(everyone).map(\.id).sorted(), ["1", "2", "3"])
        rt.followed = nil
        XCTAssertEqual(rt.filteredRecords(following).map(\.id), [], "読めていない間は空")
    }

    func testFollowButtonsMatchState() throws {
        let dsl = try timeline()
        let buttons = try XCTUnwrap(dsl.screen("post_form")).allComponents.filter { $0.action?.type.isFollow == true }
        let follow = try XCTUnwrap(buttons.first { $0.action?.type == .follow }?.action)
        let unfollow = try XCTUnwrap(buttons.first { $0.action?.type == .unfollow }?.action)
        let rt = TimelineRuntime(dsl: dsl, posts: posts, followed: ["alice"])

        XCTAssertFalse(rt.showsFollowButton(follow, recordId: "1"), "フォロー済み: follow を隠す")
        XCTAssertTrue(rt.showsFollowButton(unfollow, recordId: "1"))
        XCTAssertTrue(rt.showsFollowButton(follow, recordId: "2"), "まだ: follow を出す")
        XCTAssertFalse(rt.showsFollowButton(unfollow, recordId: "2"))
        XCTAssertFalse(rt.showsFollowButton(follow, recordId: "3"), "自分の投稿では両方隠す")
        XCTAssertFalse(rt.showsFollowButton(unfollow, recordId: "3"))
        XCTAssertFalse(rt.showsFollowButton(follow, recordId: nil), "新規の投稿フォーム")
        XCTAssertTrue(rt.showsFollowButton(Action(type: .back), recordId: nil), "ほかのアクションは関係ない")
    }
}
