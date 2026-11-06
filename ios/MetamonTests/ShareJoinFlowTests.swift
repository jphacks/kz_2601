#if !APP_STORE_REVIEW
import XCTest
import MetamonDSL
import MetamonServices
@testable import Metamon

/// プレビュー・import・参加・インストールの成否と呼び出しを記録するシェアの API(#558)。
private final class FakeShareClient: SharingClient, MembershipClient, @unchecked Sendable {
    private let lock = NSLock()
    var preview: Result<ShareLinkPreview, Error>?
    var imported: Result<InstalledApp, Error> = .success(InstalledApp(appId: "copied-app", name: "家計簿"))
    var joined: Result<InstalledApp, Error> = .success(InstalledApp(appId: "family-app", name: "家族の買い物", joined: true, member: true))
    var installed: Result<InstalledApp, Error> = .success(InstalledApp(appId: "catalog-walk", name: "散歩"))
    private(set) var previewCalls: [String] = []
    private(set) var importCalls: [String] = []
    private(set) var joinCalls: [(token: String, displayName: String)] = []
    private(set) var installCalls: [String] = []

    func previewShare(token: String) async throws -> ShareLinkPreview {
        lock.withLock { previewCalls.append(token) }
        guard let preview else { throw URLError(.notConnectedToInternet) }
        return try preview.get()
    }
    func importShare(token: String) async throws -> InstalledApp {
        lock.withLock { importCalls.append(token) }
        return try imported.get()
    }
    func importShare(token: String, displayName: String) async throws -> InstalledApp {
        lock.withLock { joinCalls.append((token, displayName)) }
        return try joined.get()
    }
    func install(publicAppId: String) async throws -> InstalledApp {
        lock.withLock { installCalls.append(publicAppId) }
        return try installed.get()
    }

    func share(appId: String) async throws -> ShareInfo { throw URLError(.badURL) }
    func revokeShare(token: String) async throws {}
    func publish(appId: String, description: String, tags: [String], authorName: String?) async throws -> PublicAppListing { PublicAppListing(id: appId, name: "n") }
    func unpublish(appId: String) async throws {}
    func searchPublicApps(query: String, sort: String) async throws -> [PublicAppListing] { [] }
    func hasMembersOnlyData(appId: String) async throws -> Bool { true }
    func members(appId: String) async throws -> SharedMemberList { SharedMemberList(items: [], role: .member) }
    func removeMember(appId: String, uid: String) async throws {}
    func leave(appId: String) async throws {}
    func setAdmin(appId: String, uid: String, admin: Bool) async throws {}
}

/// 参加者 API を持たない古いシェアの実装(プレビュー無しで import する経路)。
private final class PlainShareClient: SharingClient, @unchecked Sendable {
    var imported = InstalledApp(appId: "plain-app", name: "日記")
    func importShare(token: String) async throws -> InstalledApp { imported }
    func install(publicAppId: String) async throws -> InstalledApp { imported }
    func share(appId: String) async throws -> ShareInfo { throw URLError(.badURL) }
    func revokeShare(token: String) async throws {}
    func publish(appId: String, description: String, tags: [String], authorName: String?) async throws -> PublicAppListing { PublicAppListing(id: appId, name: "n") }
    func unpublish(appId: String) async throws {}
    func searchPublicApps(query: String, sort: String) async throws -> [PublicAppListing] { [] }
}

/// #558: 共有リンクの取り込み(importShared)→ 課金画面(#414)→ 参加の確認(pendingJoin)→ 参加(joinShared)と、公開アプリのインストール(installPublic)の分かれ道。
@MainActor
final class ShareJoinFlowTests: XCTestCase {
    private let token = "tok_family"
    private let familyPreview = ShareLinkPreview(token: "tok_family", name: "家族の買い物", sharedData: true)
    private var savedDefaults: [String: Any?] = [:]
    private let defaultsKeys = [SharedDisplayName.key, "metamon.appId"]

    override func setUp() {
        super.setUp()
        savedDefaults = Dictionary(uniqueKeysWithValues: defaultsKeys.map { ($0, UserDefaults.standard.object(forKey: $0)) })
        UserDefaults.standard.removeObject(forKey: SharedDisplayName.key)
        resetSharingUI()
    }

    override func tearDown() {
        for (key, value) in savedDefaults {
            if let value { UserDefaults.standard.set(value, forKey: key) } else { UserDefaults.standard.removeObject(forKey: key) }
        }
        resetSharingUI()
        super.tearDown()
    }

    private func resetSharingUI() {
        SharingUI.shared.pendingJoin = nil
        SharingUI.shared.showDiscover = false
        SharingUI.shared.issued = [:]
        SharingUI.shared.shareTarget = nil
    }

    // 文言は本体と同じく名前を変数で埋める(リテラルを埋めるとキーが変わり翻訳されない)
    private func added(_ name: String) -> String { L10n.tr("シェアされた「\(name)」を追加しました") }
    private func installed(_ name: String) -> String { L10n.tr("「\(name)」をインストールしました") }
    private func joinedBanner(_ name: String) -> String { L10n.tr("「\(name)」に参加しました") }
    private func declined(_ name: String) -> String { L10n.tr("「\(name)」には参加しませんでした。参加には Tailor It Pro が必要です") }

    private func makeRuntime(_ sharing: (any SharingClient)?, enforced: Bool = true, pro: Bool = false) async throws -> AppRuntime {
        var services = BuildConfig.make(environment: .mock)
        services.sharing = sharing
        let rt = AppRuntime(document: try DSLExamples.document("kakeibo", version: 1), services: services)
        let defaults = UserDefaults(suiteName: "ShareJoinFlowTests.\(UUID().uuidString)")!
        rt.proGate = ProGate(store: ProEntitlementStore(mode: .mock), usage: CreationUsageStore(defaults: defaults),
                             quota: CreationQuota(dailyFreeLimit: 100), limitDisabled: !enforced)
        if pro { _ = try await rt.proGate.store.purchaseOffer() }
        return rt
    }

    private func waitForPaywall(_ rt: AppRuntime, timeout: Duration = .seconds(5)) async throws -> PaywallRequest {
        let deadline = ContinuousClock.now + timeout
        while ContinuousClock.now < deadline {
            if let request = rt.proGate.paywall { return request }
            try await Task.sleep(for: .milliseconds(20))
        }
        XCTFail("課金画面が出ない")
        throw Timeout()
    }

    private struct Timeout: Error {}

    private func buy(_ rt: AppRuntime) async throws {
        let outcome = try await rt.proGate.store.purchaseOffer()
        XCTAssertEqual(outcome, .purchased)
        rt.proGate.finishPaywall() // 購入後に課金画面が自動で閉じる
    }

    // MARK: - importShared: 記録を共有するアプリ(プレビューあり)

    /// 自分のでない共有アプリ → 課金画面(参加の理由・閉じた時の一言)→ 購入 → 参加の確認。この時点では import しない
    func testSharedPreviewPaywallPurchaseOpensJoinConfirmation() async throws {
        let fake = FakeShareClient()
        fake.preview = .success(familyPreview)
        let rt = try await makeRuntime(fake)
        let run = Task { await rt.importShared(token: token) }

        let request = try await waitForPaywall(rt)
        XCTAssertEqual(request.reason, .join(appName: "家族の買い物"))
        XCTAssertEqual(request.placement, .root)
        XCTAssertEqual(request.closeNote, L10n.tr("閉じると参加しません。"))
        XCTAssertNil(SharingUI.shared.pendingJoin, "課金画面の間は参加の確認を出さない")

        try await buy(rt)
        await run.value
        XCTAssertEqual(SharingUI.shared.pendingJoin, PendingJoin(token: token, preview: familyPreview))
        XCTAssertTrue(fake.importCalls.isEmpty && fake.joinCalls.isEmpty, "参加の確認の前に import しない")
        XCTAssertNil(rt.proGate.paywall)
        XCTAssertNil(rt.toast)
    }

    /// 課金画面を閉じる → 参加の確認を出さず、import もしない。理由をトーストで知らせる
    func testSharedPreviewPaywallDismissDoesNotJoin() async throws {
        let fake = FakeShareClient()
        fake.preview = .success(familyPreview)
        let rt = try await makeRuntime(fake)
        let appId = rt.document.appId
        let run = Task { await rt.importShared(token: token) }

        _ = try await waitForPaywall(rt)
        rt.proGate.finishPaywall()
        await run.value
        XCTAssertNil(SharingUI.shared.pendingJoin)
        XCTAssertTrue(fake.importCalls.isEmpty && fake.joinCalls.isEmpty)
        XCTAssertEqual(rt.toast, declined("家族の買い物"))
        XCTAssertEqual(rt.document.appId, appId, "変身しない")
    }

    /// Pro ならそのまま参加の確認(課金画面は出さない)
    func testSharedPreviewForProSkipsPaywall() async throws {
        let fake = FakeShareClient()
        fake.preview = .success(familyPreview)
        let rt = try await makeRuntime(fake, pro: true)
        await rt.importShared(token: token)
        XCTAssertNil(rt.proGate.paywall)
        XCTAssertEqual(SharingUI.shared.pendingJoin?.token, token)
        XCTAssertTrue(fake.importCalls.isEmpty)
    }

    /// 課金の制限が無効(デモ端末)でも参加の確認は出す(課金画面だけ出さない)
    func testSharedPreviewNotEnforcedSkipsPaywallButAsksToJoin() async throws {
        let fake = FakeShareClient()
        fake.preview = .success(familyPreview)
        let rt = try await makeRuntime(fake, enforced: false)
        await rt.importShared(token: token)
        XCTAssertNil(rt.proGate.paywall)
        XCTAssertEqual(SharingUI.shared.pendingJoin?.preview, familyPreview)
    }

    // MARK: - importShared: そのまま取り込む

    /// 記録を共有しないアプリ → プレビューの後そのまま import して変身(課金画面・参加の確認なし)
    func testPrivateAppIsImportedDirectly() async throws {
        let fake = FakeShareClient()
        fake.preview = .success(ShareLinkPreview(token: token, name: "家計簿", sharedData: false))
        let rt = try await makeRuntime(fake)
        await rt.importShared(token: token)

        XCTAssertEqual(fake.previewCalls, [token])
        XCTAssertEqual(fake.importCalls, [token])
        XCTAssertTrue(fake.joinCalls.isEmpty)
        XCTAssertNil(SharingUI.shared.pendingJoin)
        XCTAssertNil(rt.proGate.paywall)
        XCTAssertEqual(rt.document.appId, "copied-app")
        XCTAssertEqual(rt.summaryBanner?.text, added("家計簿"))
    }

    /// プレビューが取れない(古いサーバー・通信の失敗)→ 従来どおり import
    func testPreviewFailureFallsBackToImport() async throws {
        let fake = FakeShareClient()
        fake.preview = .failure(URLError(.timedOut))
        let rt = try await makeRuntime(fake)
        await rt.importShared(token: token)
        XCTAssertEqual(fake.importCalls, [token])
        XCTAssertNil(SharingUI.shared.pendingJoin)
        XCTAssertEqual(rt.document.appId, "copied-app")
    }

    /// 参加者 API の無いシェアの実装はプレビューを見ずに import
    func testClientWithoutMembershipImportsDirectly() async throws {
        let rt = try await makeRuntime(PlainShareClient())
        await rt.importShared(token: token)
        XCTAssertNil(SharingUI.shared.pendingJoin)
        XCTAssertEqual(rt.document.appId, "plain-app")
        XCTAssertEqual(rt.summaryBanner?.text, added("日記"))
    }

    /// プレビューが取れずに import した結果が共有アプリ(joined)→ 変身の前に課金画面 → 閉じると変身しない
    func testImportedJoinedAppWithoutPreviewGoesThroughPaywall() async throws {
        let fake = FakeShareClient()
        fake.imported = .success(InstalledApp(appId: "board-app", name: "掲示板", joined: true))
        let rt = try await makeRuntime(fake)
        let appId = rt.document.appId
        let run = Task { await rt.importShared(token: token) }

        let request = try await waitForPaywall(rt)
        XCTAssertEqual(request.reason, .join(appName: "掲示板"))
        rt.proGate.finishPaywall()
        await run.value
        XCTAssertEqual(rt.document.appId, appId, "買わなければ開かない")
        XCTAssertEqual(rt.toast, declined("掲示板"))
        XCTAssertNil(rt.summaryBanner)
    }

    /// 同じ経路で購入 → 変身し、みんなの投稿が見えることを添える
    func testImportedJoinedAppPurchaseSwitches() async throws {
        let fake = FakeShareClient()
        fake.imported = .success(InstalledApp(appId: "board-app", name: "掲示板", joined: true))
        let rt = try await makeRuntime(fake)
        let run = Task { await rt.importShared(token: token) }
        _ = try await waitForPaywall(rt)
        try await buy(rt)
        await run.value
        XCTAssertEqual(rt.document.appId, "board-app")
        XCTAssertEqual(rt.summaryBanner?.text, added("掲示板") + L10n.tr("。みんなの投稿も見られます"))
    }

    /// 自分が出した共有アプリのリンクを自分で開く(preview.mine)。`PaidGate.joinNeedsPaywall` の方針どおり自分のアプリは無料のはず
    func testOwnSharedAppLinkNeverShowsPaywall() async throws {
        let fake = FakeShareClient()
        fake.preview = .success(ShareLinkPreview(token: token, name: "家族の買い物", mine: true, sharedData: true))
        // backend installCopy: 共有 entity を持つアプリは元 appId を返し joined:true(既に持っていても)
        fake.imported = .success(InstalledApp(appId: "family-app", name: "家族の買い物", joined: true))
        let rt = try await makeRuntime(fake)
        let run = Task { await rt.importShared(token: token) }

        let deadline = ContinuousClock.now + .seconds(2)
        while rt.proGate.paywall == nil, ContinuousClock.now < deadline { try await Task.sleep(for: .milliseconds(20)) }
        // #561: adopt() は InstalledApp.joined を見るが、自分のアプリ(preview.mine)は無料
        XCTAssertNil(rt.proGate.paywall, "自分のアプリを開くのに課金画面を出さない")
        rt.proGate.finishPaywall()
        await run.value
        XCTAssertNil(SharingUI.shared.pendingJoin, "自分のアプリは参加の確認を出さない")
        XCTAssertTrue(fake.joinCalls.isEmpty)
        XCTAssertEqual(fake.importCalls, [token])
        XCTAssertEqual(rt.document.appId, "family-app")
        XCTAssertNil(rt.toast)
    }

    /// #565: プレビューが取れずに自分の共有アプリのリンクを開く → 取り込み結果の mine で課金画面を出さない
    func testOwnLinkWithoutPreviewSkipsPaywallByInstalledMine() async throws {
        let fake = FakeShareClient()
        fake.preview = .failure(URLError(.timedOut))
        fake.imported = .success(InstalledApp(appId: "family-app", name: "家族の買い物", joined: true, mine: true))
        let rt = try await makeRuntime(fake)
        let run = Task { await rt.importShared(token: token) }

        let deadline = ContinuousClock.now + .seconds(2)
        while rt.proGate.paywall == nil, ContinuousClock.now < deadline { try await Task.sleep(for: .milliseconds(20)) }
        XCTAssertNil(rt.proGate.paywall, "プレビューが無くても自分のアプリに課金画面を出さない")
        rt.proGate.finishPaywall()
        await run.value
        XCTAssertEqual(fake.importCalls, [token])
        XCTAssertNil(SharingUI.shared.pendingJoin)
        XCTAssertEqual(rt.document.appId, "family-app")
        XCTAssertNil(rt.toast)
    }

    // MARK: - importShared: 失敗

    func testImportFailureShowsRevokedMessage() async throws {
        let fake = FakeShareClient()
        fake.preview = .success(ShareLinkPreview(token: token, name: "家計簿"))
        fake.imported = .failure(APIError.server(status: 404, code: "not_found", message: "share not found"))
        let rt = try await makeRuntime(fake)
        let appId = rt.document.appId
        await rt.importShared(token: token)
        XCTAssertEqual(rt.toast, L10n.tr("このシェアリンクは取り消されたか、存在しません"))
        XCTAssertEqual(rt.document.appId, appId)
    }

    func testImportWithoutServerShowsToast() async throws {
        let rt = try await makeRuntime(nil)
        await rt.importShared(token: token)
        XCTAssertEqual(rt.toast, L10n.tr("シェアリンクはサーバー接続時のみ開けます"))
        XCTAssertNil(SharingUI.shared.pendingJoin)
    }

    func testOpenShareURL() async throws {
        let fake = FakeShareClient()
        fake.preview = .success(familyPreview)
        let rt = try await makeRuntime(fake, pro: true)
        let other = await rt.openShareURL(URL(string: "https://example.com/import/tok_family")!)
        XCTAssertFalse(other, "tailorit://import 以外は扱わない")
        XCTAssertTrue(fake.previewCalls.isEmpty)

        let handled = await rt.openShareURL(URL(string: "tailorit://import/\(token)")!)
        XCTAssertTrue(handled)
        XCTAssertEqual(fake.previewCalls, [token])
        XCTAssertEqual(SharingUI.shared.pendingJoin?.token, token)
    }

    // MARK: - joinShared

    /// 参加 → 表示名を付けて import・シートを閉じて変身・表示名を次回の初期値に覚える
    func testJoinSuccessClosesSheetAndSwitches() async throws {
        let fake = FakeShareClient()
        let rt = try await makeRuntime(fake, pro: true)
        let join = PendingJoin(token: token, preview: familyPreview)
        SharingUI.shared.pendingJoin = join

        let error = await rt.joinShared(join, displayName: "  父  ")
        XCTAssertNil(error)
        XCTAssertEqual(fake.joinCalls.map(\.token), [token])
        XCTAssertEqual(fake.joinCalls.first?.displayName, "  父  ")
        XCTAssertTrue(fake.importCalls.isEmpty, "参加は表示名つきの import")
        XCTAssertNil(SharingUI.shared.pendingJoin, "シートを閉じる")
        XCTAssertEqual(rt.document.appId, "family-app")
        XCTAssertEqual(rt.summaryBanner?.text, joinedBanner("家族の買い物") + L10n.tr("。参加している人の記録も見られます"))
        XCTAssertEqual(SharedDisplayName.last, "父", "前後の空白を除いて覚える")
    }

    /// 参加に失敗 → シートは開いたまま、画面に出す文言を返す(変身しない)
    func testJoinFailureKeepsSheetOpenAndReturnsMessage() async throws {
        let fake = FakeShareClient()
        let denied = APIError.server(status: 403, code: "forbidden", message: "the person who sent this link no longer participates in the app",
                                     details: ["reason": .string("inviter_left")])
        fake.joined = .failure(denied)
        let rt = try await makeRuntime(fake, pro: true)
        let appId = rt.document.appId
        let join = PendingJoin(token: token, preview: familyPreview)
        SharingUI.shared.pendingJoin = join

        let error = await rt.joinShared(join, displayName: "父")
        XCTAssertEqual(error, SharedAccessDenial.inviterLeft.message)
        XCTAssertEqual(SharingUI.shared.pendingJoin, join, "シートは開いたまま")
        XCTAssertEqual(rt.document.appId, appId)
        XCTAssertNil(rt.summaryBanner)

        fake.joined = .failure(APIError.server(status: 404, code: "not_found", message: "share not found"))
        let revoked = await rt.joinShared(join, displayName: "")
        XCTAssertEqual(revoked, L10n.tr("このシェアリンクは取り消されたか、存在しません"))
        XCTAssertEqual(SharingUI.shared.pendingJoin, join)
    }

    func testJoinWithoutMembershipClientReturnsMessage() async throws {
        let rt = try await makeRuntime(PlainShareClient())
        let join = PendingJoin(token: token, preview: familyPreview)
        SharingUI.shared.pendingJoin = join
        let error = await rt.joinShared(join, displayName: "父")
        XCTAssertEqual(error, L10n.tr("シェアリンクはサーバー接続時のみ開けます"))
        XCTAssertEqual(SharingUI.shared.pendingJoin, join)
        XCTAssertEqual(SharedDisplayName.last, "", "参加できない時は表示名を覚えない")
    }

    /// 空の表示名は覚えない(前回の名前を消さない)
    func testJoinWithBlankNameKeepsPreviousName() async throws {
        UserDefaults.standard.set("たろう", forKey: SharedDisplayName.key)
        let rt = try await makeRuntime(FakeShareClient(), pro: true)
        let error = await rt.joinShared(PendingJoin(token: token, preview: familyPreview), displayName: "   ")
        XCTAssertNil(error)
        XCTAssertEqual(SharedDisplayName.last, "たろう")
    }

    /// 課金画面で購入 → 参加の確認 → 参加、の通し
    func testPaywallThenJoinEndToEnd() async throws {
        let fake = FakeShareClient()
        fake.preview = .success(familyPreview)
        let rt = try await makeRuntime(fake)
        let run = Task { await rt.importShared(token: token) }
        _ = try await waitForPaywall(rt)
        try await buy(rt)
        await run.value

        let join = try XCTUnwrap(SharingUI.shared.pendingJoin)
        let error = await rt.joinShared(join, displayName: "父")
        XCTAssertNil(error)
        XCTAssertNil(rt.proGate.paywall, "購入済みなので参加の後に聞き直さない")
        XCTAssertNil(SharingUI.shared.pendingJoin)
        XCTAssertEqual(rt.document.appId, "family-app")
    }

    // MARK: - installPublic

    func testInstallPublicSuccessSwitches() async throws {
        let fake = FakeShareClient()
        let rt = try await makeRuntime(fake)
        await rt.installPublic(PublicAppListing(id: "catalog-walk", name: "散歩"))
        XCTAssertEqual(fake.installCalls, ["catalog-walk"])
        XCTAssertNil(rt.proGate.paywall, "共有の無いアプリは課金画面を出さない")
        XCTAssertEqual(rt.document.appId, "catalog-walk")
        XCTAssertEqual(rt.summaryBanner?.text, installed("散歩"))
        XCTAssertNil(rt.toast)
    }

    func testInstallPublicFailureShowsToast() async throws {
        let fake = FakeShareClient()
        let failure = URLError(.notConnectedToInternet)
        fake.installed = .failure(failure)
        let rt = try await makeRuntime(fake)
        let appId = rt.document.appId
        await rt.installPublic(PublicAppListing(id: "catalog-walk", name: "散歩"))
        XCTAssertEqual(rt.toast, L10n.tr("インストールできませんでした: \(failure.localizedDescription)"))
        XCTAssertEqual(rt.document.appId, appId)
        XCTAssertNil(rt.summaryBanner)
    }

    func testInstallPublicWithoutServerDoesNothing() async throws {
        let rt = try await makeRuntime(nil)
        let appId = rt.document.appId
        await rt.installPublic(PublicAppListing(id: "catalog-walk", name: "散歩"))
        XCTAssertNil(rt.toast)
        XCTAssertEqual(rt.document.appId, appId)
    }

    /// みんなの記録を見る公開アプリ(joined)→ 探すを閉じて課金画面 → 閉じたら開かない
    func testInstallPublicJoinedAppPaywallDismiss() async throws {
        let fake = FakeShareClient()
        fake.installed = .success(InstalledApp(appId: "catalog-board", name: "掲示板", joined: true))
        let rt = try await makeRuntime(fake)
        let appId = rt.document.appId
        SharingUI.shared.showDiscover = true
        let run = Task { await rt.installPublic(PublicAppListing(id: "catalog-board", name: "掲示板")) }

        let request = try await waitForPaywall(rt)
        XCTAssertEqual(request.reason, .join(appName: "掲示板"))
        XCTAssertFalse(SharingUI.shared.showDiscover, "課金画面の前に探すを閉じる")
        rt.proGate.finishPaywall()
        await run.value
        XCTAssertEqual(rt.document.appId, appId)
        XCTAssertEqual(rt.toast, declined("掲示板"))
    }

    func testInstallPublicJoinedAppPaywallPurchase() async throws {
        let fake = FakeShareClient()
        fake.installed = .success(InstalledApp(appId: "catalog-board", name: "掲示板", joined: true))
        let rt = try await makeRuntime(fake)
        let run = Task { await rt.installPublic(PublicAppListing(id: "catalog-board", name: "掲示板")) }
        _ = try await waitForPaywall(rt)
        try await buy(rt)
        await run.value
        XCTAssertEqual(rt.document.appId, "catalog-board")
        XCTAssertEqual(rt.summaryBanner?.text, installed("掲示板") + L10n.tr("。みんなの投稿も見られます"))
    }

    /// 自分が公開した、みんなの記録を見るアプリ(listing.mine・joined)は課金画面を出さずに開く(#561)
    func testInstallPublicOwnJoinedAppSkipsPaywall() async throws {
        let fake = FakeShareClient()
        fake.installed = .success(InstalledApp(appId: "catalog-board", name: "掲示板", joined: true))
        let rt = try await makeRuntime(fake)
        await rt.installPublic(PublicAppListing(id: "catalog-board", name: "掲示板", mine: true))
        XCTAssertNil(rt.proGate.paywall, "自分のアプリは無料")
        XCTAssertEqual(rt.document.appId, "catalog-board")
        XCTAssertEqual(rt.summaryBanner?.text, installed("掲示板") + L10n.tr("。みんなの投稿も見られます"))
        XCTAssertNil(rt.toast)
    }

    /// 既にそのアプリを開いている時は切り替えずにバナーだけ出す
    func testInstallPublicAlreadyCurrentAppOnlyShowsBanner() async throws {
        let fake = FakeShareClient()
        let rt = try await makeRuntime(fake)
        fake.installed = .success(InstalledApp(appId: rt.document.appId, name: "家計簿"))
        await rt.installPublic(PublicAppListing(id: "catalog-kakeibo", name: "家計簿"))
        XCTAssertNil(rt.morphJob)
        XCTAssertEqual(rt.summaryBanner?.text, installed("家計簿"))
    }
}
#endif
