#if !APP_STORE_REVIEW
import XCTest
import MetamonDSL
import MetamonServices
import MetamonRenderer
@testable import Metamon

/// 送った操作を記録する仲間探しの API(成否を切り替えられる)。
private final class FakeMatching: MatchingClient, @unchecked Sendable {
    private let lock = NSLock()
    var error: Error?
    var result: MatchInterest?
    private(set) var calls: [String] = []

    private func log(_ s: String) throws {
        lock.withLock { calls.append(s) }
        if let error { throw error }
    }
    func sendInterest(appId: String, recordId: String, fromRecordId: String, ageConfirmed: Bool) async throws -> MatchInterest {
        try log("send \(recordId) from=\(fromRecordId) age=\(ageConfirmed)")
        return result ?? MatchInterest(uid: "u-hanako", profileRecordId: recordId)
    }
    func list(appId: String, box: MatchBox) async throws -> [MatchInterest] {
        try log("list \(box.rawValue)")
        return box == .incoming ? [MatchInterest(uid: "u-taro", profileRecordId: "r-taro")] : []
    }
    func accept(appId: String, uid: String, ageConfirmed: Bool) async throws -> MatchInterest {
        try log("accept \(uid) age=\(ageConfirmed)")
        return MatchInterest(uid: uid, profileRecordId: "r-\(uid)", status: "matched")
    }
    func decline(appId: String, uid: String) async throws { try log("decline \(uid)") }
    func unmatch(appId: String, uid: String) async throws { try log("unmatch \(uid)") }
}

/// #507 phase 2: 「つながりたい」の出し分け・18 歳以上の確認(初回だけ)・失敗の理由。
@MainActor
final class MatchingFlowTests: XCTestCase {
    private let me = MockAuthService.fixedUID
    private let hanako = Record(id: "r-hanako", data: ["hobby": "将棋"], author: "u-hanako", authorName: "はなこ")
    private var defaults: UserDefaults!

    override func setUp() {
        super.setUp()
        defaults = UserDefaults(suiteName: "MatchingFlowTests")
        defaults.removePersistentDomain(forName: "MatchingFlowTests")
        MatchingUI.shared.reset()
        MatchingUI.shared.ageGate = MatchingAgeGate(defaults: defaults)
        ModerationUI.shared.blocked = [:]
    }

    private func runtime(_ fake: FakeMatching?, withProfile: Bool = true) async throws -> AppRuntime {
        let json = """
        {"schemaVersion":1,"app":{"name":"仲間さがし","voiceEntry":"floating"},"initialScreen":"home",
         "entities":[{"id":"profile","name":"プロフィール","scope":"public","match":"profile","fields":[{"id":"hobby","name":"趣味","type":"string"}]},
                     {"id":"contact","name":"連絡先","scope":"public","match":"matched","fields":[{"id":"how","name":"連絡方法","type":"string"}]}],
         "screens":[{"id":"home","title":"さがす","components":[{"type":"list","entity":"profile","item":{"title":"hobby","interest":true}}]}]}
        """
        let dsl = try JSONDecoder().decode(AppDSL.self, from: Data(json.utf8))
        var services = BuildConfig.make(environment: .mock)
        services.matching = fake
        let rt = AppRuntime(document: AppDocument(appId: "nakama-\(UUID().uuidString.prefix(6))", version: 1, dsl: dsl), services: services)
        if withProfile { _ = await rt.createRecord(entity: "profile", data: ["hobby": "囲碁"]) }
        return rt
    }

    func testButtonOnlyOnOthersProfiles() async throws {
        let rt = try await runtime(FakeMatching())
        XCTAssertEqual(rt.interestState(entity: "profile", record: hanako), InterestState.none)
        let mine = try XCTUnwrap(rt.records(for: "profile").first)
        XCTAssertNil(rt.interestState(entity: "profile", record: mine), "自分の行には出さない")
        XCTAssertNil(rt.interestState(entity: "contact", record: hanako), "プロフィールでない entity")
        let mock = try await runtime(nil)
        XCTAssertNil(mock.interestState(entity: "profile", record: hanako), "仲間探しの無い版(モック)")
        ModerationUI.shared.blocked = ["u-hanako": "はなこ"]
        XCTAssertNil(rt.interestState(entity: "profile", record: hanako), "ブロックした人")
    }

    /// 初回は 18 歳以上の確認を出して送らない → 確認したら送る → 2 回目からは聞かない。
    func testAdultConfirmationOnceThenSends() async throws {
        let fake = FakeMatching()
        let rt = try await runtime(fake)
        await rt.sendInterest(entity: "profile", record: hanako)
        XCTAssertTrue(fake.calls.isEmpty, "確認の前は送らない")
        let pending = try XCTUnwrap(MatchingUI.shared.askingAge)

        await rt.confirmAdult(pending)
        XCTAssertNil(MatchingUI.shared.askingAge)
        XCTAssertTrue(MatchingUI.shared.ageGate.isConfirmed)
        let mine = try XCTUnwrap(rt.records(for: "profile").first)
        XCTAssertEqual(fake.calls, ["send r-hanako from=\(mine.id) age=true"])
        XCTAssertEqual(rt.interestState(entity: "profile", record: hanako), .sent)

        await rt.acceptInterest(MatchInterest(uid: "u-taro", profileRecordId: "r-taro"))
        XCTAssertNil(MatchingUI.shared.askingAge, "2 回目からは聞かない")
        XCTAssertEqual(fake.calls.last, "accept u-taro age=true")
        XCTAssertEqual(rt.toast, L10n.tr("つながりました。連絡先が見られます"))
    }

    func testUnderageSendsNothing() async throws {
        let fake = FakeMatching()
        let rt = try await runtime(fake)
        await rt.sendInterest(entity: "profile", record: hanako)
        rt.declineAdult()
        XCTAssertNil(MatchingUI.shared.askingAge)
        XCTAssertFalse(MatchingUI.shared.ageGate.isConfirmed, "18 歳未満は覚えない(次も聞く)")
        XCTAssertTrue(fake.calls.isEmpty)
        XCTAssertEqual(rt.toast, L10n.tr("18 歳未満の方は「つながりたい」を使えません"))
    }

    /// サーバーが確認を求めたら(400 age_unconfirmed)覚えた確認を消して聞き直す。
    func testServerAskingAgainResetsTheGate() async throws {
        let fake = FakeMatching()
        fake.error = APIError.server(status: 400, code: "bad_request", message: "confirm", details: ["reason": .string("age_unconfirmed")])
        MatchingUI.shared.ageGate.confirm()
        let rt = try await runtime(fake)
        await rt.sendInterest(entity: "profile", record: hanako)
        XCTAssertFalse(MatchingUI.shared.ageGate.isConfirmed)
        XCTAssertNotNil(MatchingUI.shared.askingAge)
        XCTAssertEqual(rt.interestState(entity: "profile", record: hanako), InterestState.none, "失敗したら戻す")
    }

    func testFailureTextsAndRollback() async throws {
        let fake = FakeMatching()
        MatchingUI.shared.ageGate.confirm()
        let rt = try await runtime(fake)
        fake.error = URLError(.notConnectedToInternet)
        await rt.sendInterest(entity: "profile", record: hanako)
        XCTAssertEqual(rt.toast, L10n.tr("オフラインのあいだは送れません"))
        XCTAssertEqual(rt.interestState(entity: "profile", record: hanako), InterestState.none)

        fake.error = APIError.server(status: 429, code: "rate_limited", message: "too many interests today")
        await rt.sendInterest(entity: "profile", record: hanako)
        XCTAssertEqual(rt.toast, L10n.tr("今日はこれ以上送れません。明日またお試しください"))

        fake.error = APIError.server(status: 400, code: "bad_request", message: "unblock", details: ["reason": .string("blocked")])
        await rt.sendInterest(entity: "profile", record: hanako)
        XCTAssertEqual(rt.toast, L10n.tr("ブロックを解除してから送れます"))
    }

    func testNeedsOwnProfileFirst() async throws {
        let fake = FakeMatching()
        MatchingUI.shared.ageGate.confirm()
        let rt = try await runtime(fake, withProfile: false)
        await rt.sendInterest(entity: "profile", record: hanako)
        XCTAssertTrue(fake.calls.isEmpty)
        XCTAssertEqual(rt.toast, L10n.tr("先に自分のプロフィールを書いてください"))
    }

    func testListsHideBlockedPeople() async throws {
        let rt = try await runtime(FakeMatching())
        await rt.refreshMatches([.incoming])
        XCTAssertEqual(rt.matchItems(.incoming)?.map(\.uid), ["u-taro"])
        XCTAssertNil(rt.matchItems(.matches), "読んでいない box")
        ModerationUI.shared.blocked = ["u-taro": "たろう"]
        XCTAssertEqual(rt.matchItems(.incoming), [])
    }

    func testOptionalLiveWiring() {
        XCTAssertTrue(BuildConfig.liveWirings.count >= 7)
        var services = BuildConfig.make(environment: .mock)
        XCTAssertNil(services.matching, "モックでは出さない")
        LiveWiring.matching.apply(&services, URL(string: "https://appconfig.test/"), StaticTokenProvider("t"))
        XCTAssertTrue(services.matching is HTTPMatchingClient)
    }
}
#endif
