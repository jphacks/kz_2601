import XCTest
import MetamonDSL
import MetamonRenderer
import MetamonServices
@testable import Metamon

/// サインアウト・アカウント削除(#972)。端末のデータを消す(`MacLocalWipe`)と、消す順番(`MacAccount`)
@MainActor
final class MacAccountWipeTests: XCTestCase {
    private var root: URL!
    private var suiteName: String!
    private var defaults: UserDefaults!
    private var sharedStateReset = 0
    private var notificationsRemoved = 0

    override func setUp() async throws {
        root = FileManager.default.temporaryDirectory.appendingPathComponent("metamon-wipe-test-\(UUID().uuidString)", isDirectory: true)
        suiteName = "metamon.wipe-test.\(UUID().uuidString)"
        defaults = UserDefaults(suiteName: suiteName)
        sharedStateReset = 0
        notificationsRemoved = 0
    }

    override func tearDown() async throws {
        try? FileManager.default.removeItem(at: root)
        defaults.removePersistentDomain(forName: suiteName)
    }

    private var recordsRoot: URL { root.appendingPathComponent("records", isDirectory: true) }

    /// 本物の共有の保存先・通知には触れない後始末
    private func makeWipe(engines: MacRecordEngines, library: MacAppLibrary? = nil) -> MacLocalWipe {
        var wipe = MacLocalWipe()
        wipe.records = engines
        wipe.thumbnails = MacAppThumbnailStore(directory: root.appendingPathComponent("thumbs", isDirectory: true))
        wipe.documents = DocumentCache(directory: root.appendingPathComponent("dsl", isDirectory: true))
        wipe.appList = AppListCache(directory: root.appendingPathComponent("list", isDirectory: true))
        wipe.localList = AppListCache(directory: root.appendingPathComponent("local-list", isDirectory: true))
        #if !APP_STORE_REVIEW
        wipe.feeds = FeedCache()
        wipe.social = MacSocial(ageGate: MatchingAgeGate(defaults: defaults))
        #endif
        wipe.defaults = defaults
        wipe.domain = suiteName
        wipe.library = library
        wipe.resetSharedState = { [weak self] in self?.sharedStateReset += 1 }
        wipe.removeNotifications = { [weak self] in self?.notificationsRemoved += 1 }
        return wipe
    }

    private func exists(_ url: URL) -> Bool { FileManager.default.fileExists(atPath: url.path) }

    func testWipeClearsEachStore() async throws {
        let engines = MacRecordEngines(root: recordsRoot)
        let lease = engines.acquire(appId: "a")
        _ = await lease.engine.save(appId: "a", entity: "memo", recordId: nil, data: ["n": .number(1)])
        XCTAssertTrue(exists(MacRecordVault.directory(appId: "a", root: recordsRoot)))

        let library = MacAppLibrary(store: nil)
        let wipe = makeWipe(engines: engines, library: library)
        wipe.documents.store(appId: "a", envelope: Data("{}".utf8))
        wipe.appList.store([AppSummary(id: "a", name: "メモ", currentVersion: 1)])
        let thumb = wipe.thumbnails.url("a")
        try FileManager.default.createDirectory(at: thumb.deletingLastPathComponent(), withIntermediateDirectories: true)
        try Data([1]).write(to: thumb)
        defaults.set("a", forKey: LocalAppLibrary.currentAppKey)
        defaults.set(["a"], forKey: MacAppLibrary.serverIdsKey)
        defaults.set("ja", forKey: "metamon.appLanguage")
        defaults.set(false, forKey: "metamon.crashReporting.enabled")
        defaults.set(MacTextZoom.defaultStep + 2, forKey: MacTextZoom.storageKey)
        #if !APP_STORE_REVIEW
        wipe.social.blocked["u-x"] = "x" // ブロック・参加者(#963)
        wipe.social.memberEntries["a"] = ([AppMember(uid: "u-x", displayName: "x")], true)
        wipe.social.ageGate.markSent(uid: "u-me")
        #endif

        await wipe.run()

        #if !APP_STORE_REVIEW
        XCTAssertTrue(wipe.social.blocked.isEmpty, "ブロックした人の一覧")
        XCTAssertTrue(wipe.social.memberEntries.isEmpty, "参加者")
        XCTAssertFalse(wipe.social.ageGate.isSent(uid: "u-me"), "仲間探しの申告を送った記録")
        #endif

        XCTAssertFalse(exists(recordsRoot), "記録のフォルダごと消す")
        XCTAssertNil(engines.engine(appId: "a"), "エンジンを手放す")
        XCTAssertNil(wipe.documents.loadData(appId: "a"), "作ったアプリの DSL")
        XCTAssertTrue(wipe.appList.load().isEmpty, "作ったアプリの一覧")
        XCTAssertFalse(exists(thumb), "サイドバーの写し")
        XCTAssertNil(defaults.object(forKey: LocalAppLibrary.currentAppKey), "最後に開いたアプリ")
        XCTAssertNil(defaults.object(forKey: MacAppLibrary.serverIdsKey), "サーバーにあるアプリの印")
        XCTAssertEqual(defaults.string(forKey: "metamon.appLanguage"), "ja", "表示の言語は残す(iOS と同じ)")
        XCTAssertEqual(defaults.object(forKey: "metamon.crashReporting.enabled") as? Bool, false, "送信のオプトアウトは残す")
        XCTAssertEqual(defaults.integer(forKey: MacTextZoom.storageKey), MacTextZoom.defaultStep + 2, "文字の大きさ(端末の設定)は残す")
        XCTAssertEqual(sharedStateReset, 1)
        XCTAssertEqual(notificationsRemoved, 1)
        XCTAssertEqual(library.wipeCount, 1, "開いているウインドウに知らせる")
        withExtendedLifetime(lease) {}
    }

    /// 会話の履歴(#1211)も消す。保存先が UserDefaults でも、差し替えた保存先でも残さない
    func testWipeClearsChatHistory() async throws {
        let engines = MacRecordEngines(root: recordsRoot)
        var wipe = makeWipe(engines: engines)
        let memory = ChatHistoryStore()
        memory.save([ChatHistoryEntry(kind: .user, text: "家計簿")], appId: "a")
        wipe.chatHistory = memory
        let persisted = ChatHistoryStore(defaults: defaults)
        persisted.save([ChatHistoryEntry(kind: .user, text: "メモ")], appId: "b")

        await wipe.run()

        XCTAssertTrue(memory.load(appId: "a").isEmpty)
        XCTAssertTrue(persisted.load(appId: "b").isEmpty, "UserDefaults の履歴も消える")
    }

    func testSignOutWipesThenSignsOut() async throws {
        let auth = MockAuthService(signedIn: true)
        let account = MacAccount(session: AuthSession(service: auth), account: MockAccountClient(), isLive: false)
        account.wipe = makeWipe(engines: MacRecordEngines(root: recordsRoot))
        defaults.set("a", forKey: LocalAppLibrary.currentAppKey)

        let outcome = try await account.signOutAndWipe(choice: .discardAndWipe)

        XCTAssertEqual(outcome, .signedOut)
        XCTAssertNil(auth.currentUser)
        XCTAssertNil(account.session.user)
        XCTAssertNil(defaults.object(forKey: LocalAppLibrary.currentAppKey))
        XCTAssertEqual(notificationsRemoved, 1)
        XCTAssertNil(account.busy)
    }

    func testDeleteAccountDeletesOnServerThenWipes() async throws {
        let auth = MockAuthService(signedIn: true) // 匿名なので Apple の再認証は出さない
        let server = SpyAccountClient()
        let account = MacAccount(session: AuthSession(service: auth), account: server, isLive: false)
        account.wipe = makeWipe(engines: MacRecordEngines(root: recordsRoot))
        account.appleReauth = { XCTFail("匿名は再認証しない"); return nil }

        let done = try await account.deleteAccount()

        XCTAssertTrue(done)
        XCTAssertEqual(server.calls, 1)
        XCTAssertNil(auth.currentUser)
        XCTAssertEqual(sharedStateReset, 1)
    }

    func testDeleteAccountFailureKeepsDataAndSession() async throws {
        let auth = MockAuthService(signedIn: true)
        let server = SpyAccountClient(fails: true)
        let account = MacAccount(session: AuthSession(service: auth), account: server, isLive: false)
        account.wipe = makeWipe(engines: MacRecordEngines(root: recordsRoot))
        defaults.set("a", forKey: LocalAppLibrary.currentAppKey)

        do {
            _ = try await account.deleteAccount()
            XCTFail("サーバーで消せなければ throw")
        } catch {}

        XCTAssertNotNil(auth.currentUser, "サインインしたまま(やり直せる)")
        XCTAssertEqual(defaults.string(forKey: LocalAppLibrary.currentAppKey), "a", "端末のデータは消さない")
        XCTAssertEqual(sharedStateReset, 0)
        XCTAssertNil(account.busy)
    }

    /// uid が変わった(既存の Apple アカウントへの切り替え・別の Apple ID で入り直し)時だけ前のアカウントのデータを消す
    func testUidChangeWipesPreviousAccountData() async throws {
        let account = MacAccount(session: AuthSession(service: MockAuthService(signedIn: true)), account: MockAccountClient(), isLive: false)
        account.wipe = makeWipe(engines: MacRecordEngines(root: recordsRoot))

        await account.adoptUser("anon")
        XCTAssertEqual(sharedStateReset, 0, "初めての持ち主は消さない")
        await account.adoptUser("anon")
        XCTAssertEqual(sharedStateReset, 0, "同じ uid(引き継ぎ・入り直し)は消さない")
        await account.adoptUser(nil)
        XCTAssertEqual(sharedStateReset, 0, "サインアウトの途中(nil)は消さない")

        defaults.set("a", forKey: LocalAppLibrary.currentAppKey)
        await account.adoptUser("apple")
        XCTAssertEqual(sharedStateReset, 1, "今日の残り・使っている量・書き出しを消す")
        XCTAssertEqual(notificationsRemoved, 1)
        XCTAssertNil(defaults.object(forKey: LocalAppLibrary.currentAppKey), "前のアカウントのアプリを開き直さない")
        XCTAssertEqual(defaults.string(forKey: MacAccount.ownerUidKey), "apple", "wipe の後も持ち主を残す")

        await account.adoptUser("apple")
        XCTAssertEqual(sharedStateReset, 1)
    }

    /// サインアウトの後は wipe 済みなので、次の匿名の uid で二度消さない
    func testSignOutThenNewUidDoesNotWipeTwice() async throws {
        let account = MacAccount(session: AuthSession(service: MockAuthService(signedIn: true)), account: MockAccountClient(), isLive: false)
        account.wipe = makeWipe(engines: MacRecordEngines(root: recordsRoot))
        await account.adoptUser(MockAuthService.fixedUID)

        try await account.signOutAndWipe(choice: .discardAndWipe)
        XCTAssertEqual(sharedStateReset, 1)
        await account.adoptUser("new-anon")
        XCTAssertEqual(sharedStateReset, 1, "持ち主の印も消えているので消し直さない")
    }

    /// 匿名は戻れないことを確認に出す(iOS と同じ文言)
    func testSignOutCopyWarnsAnonymous() {
        let anonymous = SignOutCopy.message(isAnonymous: true, prompt: .plain)
        let apple = SignOutCopy.message(isAnonymous: false, prompt: .plain)
        XCTAssertEqual(anonymous, SignOutCopy.anonymous)
        XCTAssertEqual(apple, SignOutCopy.apple)
        XCTAssertNotEqual(anonymous, apple)
        XCTAssertTrue(SignOutCopy.message(isAnonymous: true, prompt: .unsent(count: 2, canSend: false)).hasPrefix(anonymous))
    }
}

private final class SpyAccountClient: AccountClient, @unchecked Sendable {
    private let fails: Bool
    private(set) var calls = 0

    init(fails: Bool = false) { self.fails = fails }

    func deleteAccount() async throws {
        calls += 1
        if fails { throw URLError(.notConnectedToInternet) }
    }
}
