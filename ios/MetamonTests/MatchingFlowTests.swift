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
    /// 一覧だけ失敗させる
    var listError: Error?
    var serverAgeConfirmed = false
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
        if let listError { throw listError }
        return box == .incoming ? [MatchInterest(uid: "u-taro", profileRecordId: "r-taro")] : []
    }
    func accept(appId: String, uid: String, ageConfirmed: Bool) async throws -> MatchInterest {
        try log("accept \(uid) age=\(ageConfirmed)")
        return MatchInterest(uid: uid, profileRecordId: "r-\(uid)", status: "matched")
    }
    func decline(appId: String, uid: String) async throws { try log("decline \(uid)") }
    func unmatch(appId: String, uid: String) async throws { try log("unmatch \(uid)") }
    func ageConfirmation() async throws -> Bool {
        try log("get age")
        return serverAgeConfirmed
    }
    func confirmAge() async throws {
        try log("put age")
        serverAgeConfirmed = true
    }
}

private let ageUnconfirmed = APIError.server(status: 400, code: "bad_request", message: "confirm", details: ["reason": .string("age_unconfirmed")])

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
        XCTAssertEqual(fake.calls, ["put age", "send r-hanako from=\(mine.id) age=true"], "申告を送ってから送る")
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

    /// 仲間探しのアプリを開いた: 端末で確認済みなら申告を 1 回だけ送る(確認は出さない)。
    func testOpeningSendsTheDeviceDeclarationOnce() async throws {
        let fake = FakeMatching()
        MatchingUI.shared.ageGate.confirm()
        let rt = try await runtime(fake)
        await rt.prepareMatching()
        await rt.prepareMatching()
        XCTAssertEqual(fake.calls, ["put age"])
        XCTAssertNil(MatchingUI.shared.askingAge)
        XCTAssertTrue(MatchingUI.shared.ageGate.isSent(uid: me))
    }

    /// 端末にもサーバーにも申告が無い: 開いた時に確認を出す → 同意で PUT。
    func testOpeningWithoutDeclarationAsks() async throws {
        let fake = FakeMatching()
        let rt = try await runtime(fake)
        await rt.prepareMatching()
        XCTAssertEqual(fake.calls, ["get age"])
        let pending = try XCTUnwrap(MatchingUI.shared.askingAge)
        await rt.confirmAdult(pending)
        XCTAssertEqual(fake.calls, ["get age", "put age"])
        XCTAssertTrue(MatchingUI.shared.ageGate.isConfirmed)
        XCTAssertTrue(MatchingUI.shared.ageGate.isSent(uid: me))
    }

    /// 別の端末で申告済み: 聞かずに端末にも覚える。
    func testOpeningAdoptsTheServerDeclaration() async throws {
        let fake = FakeMatching()
        fake.serverAgeConfirmed = true
        let rt = try await runtime(fake)
        await rt.prepareMatching()
        XCTAssertNil(MatchingUI.shared.askingAge)
        XCTAssertTrue(MatchingUI.shared.ageGate.isConfirmed)
    }

    /// 「18 歳未満」と答えたら、開くたびには聞かない(ボタンを押したら聞く)。
    func testDeclinedIsNotAskedAgainOnOpen() async throws {
        let fake = FakeMatching()
        let rt = try await runtime(fake)
        await rt.prepareMatching()
        rt.declineAdult()
        await rt.prepareMatching()
        XCTAssertNil(MatchingUI.shared.askingAge)
        await rt.sendInterest(entity: "profile", record: hanako)
        XCTAssertNotNil(MatchingUI.shared.askingAge, "押した時は聞く")
    }

    /// 確認をスワイプで閉じたら「18 歳未満」と同じ: 開いた時・一覧の読み込みでは聞かず、ボタンを押したら聞く。
    func testSwipeDismissIsNotAskedAgainOnOpenOrList() async throws {
        let fake = FakeMatching()
        let rt = try await runtime(fake)
        await rt.prepareMatching()
        XCTAssertNotNil(MatchingUI.shared.askingAge)
        let toastBefore = rt.toast
        rt.dismissAdult()
        XCTAssertNil(MatchingUI.shared.askingAge)
        XCTAssertEqual(rt.toast, toastBefore, "答えていないので「使えません」は出さない")
        await rt.prepareMatching()
        XCTAssertNil(MatchingUI.shared.askingAge, "開き直しでは聞かない")
        fake.listError = ageUnconfirmed
        await rt.refreshMatches([.incoming])
        XCTAssertNil(MatchingUI.shared.askingAge, "一覧の読み込みでは聞かない")
        await rt.sendInterest(entity: "profile", record: hanako)
        guard case .send? = MatchingUI.shared.askingAge else { return XCTFail("「つながりたい」を押した時は聞く") }
        XCTAssertFalse(fake.calls.contains { $0.hasPrefix("send") }, "確認するまで送らない")
    }

    /// スワイプで閉じた後でも、ボタンからの確認で「18 歳以上」と答えれば送れる。
    func testSwipeDismissThenConfirmOnButtonSends() async throws {
        let fake = FakeMatching()
        let rt = try await runtime(fake)
        await rt.prepareMatching()
        rt.dismissAdult()
        await rt.sendInterest(entity: "profile", record: hanako)
        let pending = try XCTUnwrap(MatchingUI.shared.askingAge)
        await rt.confirmAdult(pending)
        XCTAssertFalse(MatchingUI.shared.declinedAge)
        XCTAssertTrue(fake.calls.contains { $0.hasPrefix("send") })
    }

    /// 一覧が 400 age_unconfirmed: 送った記録を消して確認を出す → 同意で PUT してから読み直す。
    func testListAgeUnconfirmedAsksThenReloads() async throws {
        let fake = FakeMatching()
        MatchingUI.shared.ageGate.confirm()
        MatchingUI.shared.ageGate.markSent(uid: me) // サーバーの記録だけ消えた
        let rt = try await runtime(fake)
        fake.listError = ageUnconfirmed
        await rt.refreshMatches([.incoming])
        XCTAssertFalse(MatchingUI.shared.ageGate.isSent(uid: me))
        let pending = try XCTUnwrap(MatchingUI.shared.askingAge)
        fake.listError = nil
        await rt.confirmAdult(pending)
        XCTAssertEqual(fake.calls, ["list incoming", "put age", "list incoming"])
        XCTAssertEqual(rt.matchItems(.incoming)?.map(\.uid), ["u-taro"])
    }

    /// プロフィールの投稿が 400 age_unconfirmed: 確認 → 同意で PUT してから投稿し直す。
    func testProfilePostRetriedAfterDeclaration() async throws {
        let fake = FakeMatching()
        let rt = try await runtime(fake, withProfile: false)
        rt.profilePostNeedsAge(entity: "profile", data: ["hobby": "囲碁"])
        let pending = try XCTUnwrap(MatchingUI.shared.askingAge)
        await rt.confirmAdult(pending)
        XCTAssertEqual(fake.calls, ["put age"])
        XCTAssertEqual(rt.records(for: "profile").first?.data["hobby"], .string("囲碁"))
    }

    /// サインアウト・アカウント削除の後片付けで、送った記録も消す。
    func testWipeForgetsTheSentDeclaration() async throws {
        MatchingUI.shared.ageGate.markSent(uid: me)
        MatchingUI.shared.wipe()
        XCTAssertFalse(MatchingUI.shared.ageGate.isSent(uid: me))
        XCTAssertFalse(LocalWipe.preservedKeys.contains(MatchingAgeGate.sentKey))
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
