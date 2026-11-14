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

/// #558: 共有リンクの取り込み(importShared)→ 課金画面(#414)→ 参加の確認(pendingJoin)→ 参加(joinShared)/ 追加の確認(pendingImport)→ 追加(confirmImport)と、公開アプリのインストール(installPublic)の分かれ道。
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
        SharingUI.shared.pendingImport = nil
        SharingUI.shared.showDiscover = false
        SharingUI.shared.issued = [:]
        SharingUI.shared.shareTarget = nil
    }

    // 文言は本体と同じく名前を変数で埋める(リテラルを埋めるとキーが変わり翻訳されない)
    private func added(_ name: String) -> String { L10n.tr("シェアされた「\(name)」を追加しました") }
    private func installed(_ name: String) -> String { L10n.tr("「\(name)」をインストールしました") }
    private func joinedBanner(_ name: String) -> String { L10n.tr("「\(name)」に参加しました") }

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

    // MARK: - importShared: 記録を共有するアプリ(プレビューあり)

    /// 自分のでない共有アプリ → 課金画面を出さずに参加の確認(参加者は無料・H5)。この時点では import しない
    func testSharedPreviewParticipantJoinsWithoutPaywall() async throws {
        let fake = FakeShareClient()
        fake.preview = .success(familyPreview)
        let rt = try await makeRuntime(fake)
        await rt.importShared(token: token)
        XCTAssertNil(rt.proGate.paywall, "参加者に課金画面を出さない")
        XCTAssertEqual(SharingUI.shared.pendingJoin, PendingJoin(token: token, preview: familyPreview))
        XCTAssertTrue(fake.importCalls.isEmpty && fake.joinCalls.isEmpty, "参加の確認の前に import しない")
        XCTAssertNil(rt.toast)
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

    // MARK: - importShared: 追加の確認 → 取り込む

    /// 記録を共有しないアプリ → 取り込む前に追加の確認(この時点では import しない・課金画面なし)
    func testPrivateAppAsksBeforeImport() async throws {
        let fake = FakeShareClient()
        let preview = ShareLinkPreview(token: token, name: "家計簿", sharedData: false)
        fake.preview = .success(preview)
        let rt = try await makeRuntime(fake)
        let appId = rt.document.appId
        await rt.importShared(token: token)

        XCTAssertEqual(fake.previewCalls, [token])
        XCTAssertEqual(SharingUI.shared.pendingImport, PendingImport(token: token, preview: preview))
        XCTAssertNil(SharingUI.shared.pendingJoin)
        XCTAssertTrue(fake.importCalls.isEmpty && fake.joinCalls.isEmpty, "追加の確認の前に import しない")
        XCTAssertNil(rt.proGate.paywall)
        XCTAssertEqual(rt.document.appId, appId)
        XCTAssertNil(rt.toast)
    }

    /// 追加の確認 → 追加する、の通し: import してシートを閉じて変身
    func testPrivateAppConfirmImportsAndSwitches() async throws {
        let fake = FakeShareClient()
        fake.preview = .success(ShareLinkPreview(token: token, name: "家計簿", sharedData: false))
        let rt = try await makeRuntime(fake)
        await rt.importShared(token: token)

        let pending = try XCTUnwrap(SharingUI.shared.pendingImport)
        let error = await rt.confirmImport(pending)
        XCTAssertNil(error)
        XCTAssertEqual(fake.importCalls, [token])
        XCTAssertTrue(fake.joinCalls.isEmpty)
        XCTAssertNil(SharingUI.shared.pendingImport, "シートを閉じる")
        XCTAssertNil(rt.proGate.paywall)
        XCTAssertEqual(rt.document.appId, "copied-app")
        XCTAssertEqual(rt.summaryBanner?.text, added("家計簿"))
    }

    /// 追加しない(シートを閉じる)→ import しない・変身しない
    func testDeclineDoesNotImport() async throws {
        let fake = FakeShareClient()
        fake.preview = .success(ShareLinkPreview(token: token, name: "家計簿", sharedData: false))
        let rt = try await makeRuntime(fake)
        let appId = rt.document.appId
        await rt.importShared(token: token)
        XCTAssertNotNil(SharingUI.shared.pendingImport)

        SharingUI.shared.pendingImport = nil
        XCTAssertTrue(fake.importCalls.isEmpty)
        XCTAssertEqual(rt.document.appId, appId)
        XCTAssertNil(rt.summaryBanner)
    }

    /// プレビューが取れない(古いサーバー・通信の失敗)→ 名前なしで追加の確認 → 追加すると import
    func testPreviewFailureStillAsksBeforeImport() async throws {
        let fake = FakeShareClient()
        fake.preview = .failure(URLError(.timedOut))
        let rt = try await makeRuntime(fake)
        await rt.importShared(token: token)
        XCTAssertEqual(SharingUI.shared.pendingImport, PendingImport(token: token, preview: nil))
        XCTAssertTrue(fake.importCalls.isEmpty)
        XCTAssertNil(SharingUI.shared.pendingJoin)

        let error = await rt.confirmImport(try XCTUnwrap(SharingUI.shared.pendingImport))
        XCTAssertNil(error)
        XCTAssertEqual(fake.importCalls, [token])
        XCTAssertEqual(rt.document.appId, "copied-app")
    }

    /// 参加者 API の無いシェアの実装もプレビューなしで追加の確認を出す
    func testClientWithoutMembershipAsksBeforeImport() async throws {
        let rt = try await makeRuntime(PlainShareClient())
        await rt.importShared(token: token)
        XCTAssertNil(SharingUI.shared.pendingJoin)
        XCTAssertEqual(SharingUI.shared.pendingImport, PendingImport(token: token, preview: nil))

        let error = await rt.confirmImport(try XCTUnwrap(SharingUI.shared.pendingImport))
        XCTAssertNil(error)
        XCTAssertEqual(rt.document.appId, "plain-app")
        XCTAssertEqual(rt.summaryBanner?.text, added("日記"))
    }

    /// プレビューが取れずに追加した結果が共有アプリ(joined)→ 課金画面を出さずに変身し、みんなの投稿が見えることを添える(H5)
    func testImportedJoinedAppWithoutPreviewSwitchesWithoutPaywall() async throws {
        let fake = FakeShareClient()
        fake.imported = .success(InstalledApp(appId: "board-app", name: "掲示板", joined: true))
        let rt = try await makeRuntime(fake)
        await rt.importShared(token: token)
        _ = await rt.confirmImport(try XCTUnwrap(SharingUI.shared.pendingImport))
        XCTAssertNil(rt.proGate.paywall, "参加者に課金画面を出さない")
        XCTAssertEqual(rt.document.appId, "board-app")
        XCTAssertEqual(rt.summaryBanner?.text, added("掲示板") + L10n.tr("。みんなの投稿も見られます"))
        XCTAssertNil(rt.toast)
    }

    /// 自分が出した共有アプリのリンクを自分で開く(preview.mine)。参加の確認・課金画面は出さず、追加の確認だけ
    func testOwnSharedAppLinkNeverShowsPaywall() async throws {
        let fake = FakeShareClient()
        fake.preview = .success(ShareLinkPreview(token: token, name: "家族の買い物", mine: true, sharedData: true))
        // backend installCopy: 共有 entity を持つアプリは元 appId を返し joined:true(既に持っていても)
        fake.imported = .success(InstalledApp(appId: "family-app", name: "家族の買い物", joined: true))
        let rt = try await makeRuntime(fake)
        await rt.importShared(token: token)
        XCTAssertNil(SharingUI.shared.pendingJoin, "自分のアプリは参加の確認を出さない")
        let pending = try XCTUnwrap(SharingUI.shared.pendingImport)
        let run = Task { await rt.confirmImport(pending) }

        let deadline = ContinuousClock.now + .seconds(2)
        while rt.proGate.paywall == nil, ContinuousClock.now < deadline { try await Task.sleep(for: .milliseconds(20)) }
        // #561: adopt() は InstalledApp.joined を見るが、自分のアプリ(preview.mine)は無料
        XCTAssertNil(rt.proGate.paywall, "自分のアプリを開くのに課金画面を出さない")
        rt.proGate.finishPaywall()
        _ = await run.value
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
        await rt.importShared(token: token)
        let pending = try XCTUnwrap(SharingUI.shared.pendingImport)
        let run = Task { await rt.confirmImport(pending) }

        let deadline = ContinuousClock.now + .seconds(2)
        while rt.proGate.paywall == nil, ContinuousClock.now < deadline { try await Task.sleep(for: .milliseconds(20)) }
        XCTAssertNil(rt.proGate.paywall, "プレビューが無くても自分のアプリに課金画面を出さない")
        rt.proGate.finishPaywall()
        _ = await run.value
        XCTAssertEqual(fake.importCalls, [token])
        XCTAssertNil(SharingUI.shared.pendingJoin)
        XCTAssertEqual(rt.document.appId, "family-app")
        XCTAssertNil(rt.toast)
    }

    // MARK: - importShared: 失敗

    /// 追加に失敗 → シートは開いたまま、画面に出す文言を返す(変身しない)
    func testImportFailureShowsRevokedMessage() async throws {
        let fake = FakeShareClient()
        fake.preview = .success(ShareLinkPreview(token: token, name: "家計簿"))
        fake.imported = .failure(APIError.server(status: 404, code: "not_found", message: "share not found"))
        let rt = try await makeRuntime(fake)
        let appId = rt.document.appId
        await rt.importShared(token: token)
        let pending = try XCTUnwrap(SharingUI.shared.pendingImport)
        let error = await rt.confirmImport(pending)
        XCTAssertEqual(error, L10n.tr("このシェアリンクは取り消されたか、存在しません"))
        XCTAssertEqual(SharingUI.shared.pendingImport, pending, "シートは開いたまま")
        XCTAssertEqual(rt.document.appId, appId)
        XCTAssertNil(rt.summaryBanner)
    }

    func testImportWithoutServerShowsToast() async throws {
        let rt = try await makeRuntime(nil)
        await rt.importShared(token: token)
        XCTAssertEqual(rt.toast, L10n.tr("シェアリンクはサーバー接続時のみ開けます"))
        XCTAssertNil(SharingUI.shared.pendingJoin)
        XCTAssertNil(SharingUI.shared.pendingImport)
    }

    /// 追加の確認の文言に英訳がある
    func testAddConfirmCopyHasEnglish() throws {
        let path = try XCTUnwrap(Bundle(for: AppRuntime.self).path(forResource: "en", ofType: "lproj"))
        let en = try XCTUnwrap(Bundle(path: path))
        for key in ["「%@」を追加しますか?", "シェアされたアプリを追加しますか?", "追加すると、このアプリに切り替わります", "追加する", "追加しない"] {
            let value = en.localizedString(forKey: key, value: "__missing__", table: nil)
            XCTAssertNotEqual(value, "__missing__", "英語が無い: \(key)")
            XCTAssertNotEqual(value, key, "英語が日本語のまま: \(key)")
        }
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

    /// #331: Universal Links `https://tailor-it.app/s/<token>` も URL スキームと同じ参加の確認に進む
    func testOpenUniversalLinkRoutesToSameJoinFlow() async throws {
        let fake = FakeShareClient()
        fake.preview = .success(familyPreview)
        let rt = try await makeRuntime(fake, pro: true)
        let handled = await rt.openShareURL(URL(string: "https://tailor-it.app/s/\(token)")!)
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

    /// 参加の確認 → 参加、の通し(課金画面は一度も出ない・H5)
    func testJoinEndToEndWithoutPaywall() async throws {
        let fake = FakeShareClient()
        fake.preview = .success(familyPreview)
        let rt = try await makeRuntime(fake)
        await rt.importShared(token: token)

        let join = try XCTUnwrap(SharingUI.shared.pendingJoin)
        let error = await rt.joinShared(join, displayName: "父")
        XCTAssertNil(error)
        XCTAssertNil(rt.proGate.paywall, "参加者に課金画面を出さない")
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
        XCTAssertEqual(rt.toast, L10n.tr("\(L10n.tr("インストールできませんでした"))。\(ErrorCopy.offlineInterruptedBody)"))
        XCTAssertFalse(rt.toast?.contains(failure.localizedDescription) ?? true, "OS のエラー文は出さない(H9)")
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

    /// みんなの記録を見る公開アプリ(joined)→ 課金画面を出さずに開く(H5)
    func testInstallPublicJoinedAppOpensWithoutPaywall() async throws {
        let fake = FakeShareClient()
        fake.installed = .success(InstalledApp(appId: "catalog-board", name: "掲示板", joined: true))
        let rt = try await makeRuntime(fake)
        await rt.installPublic(PublicAppListing(id: "catalog-board", name: "掲示板"))
        XCTAssertNil(rt.proGate.paywall, "インストールした人に課金画面を出さない")
        XCTAssertEqual(rt.document.appId, "catalog-board")
        XCTAssertEqual(rt.summaryBanner?.text, installed("掲示板") + L10n.tr("。みんなの投稿も見られます"))
        XCTAssertNil(rt.toast)
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
