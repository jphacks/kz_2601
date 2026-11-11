import XCTest
import MetamonDSL
import MetamonServices
@testable import MetamonRenderer
@testable import Metamon

/// サインアウトで端末のデータを消す(アカウント削除と同じ後片付け)・生成中は削除できない。
@MainActor
final class SignOutWipeTests: XCTestCase {
    private var savedLanguage: AppLanguage = .system

    override func setUp() {
        savedLanguage = LanguagePreference.shared.setting
        LanguagePreference.shared.setting = .ja
    }

    override func tearDown() { LanguagePreference.shared.setting = savedLanguage }

    private final class RecordingAccount: AccountClient, @unchecked Sendable {
        var called = false
        func deleteAccount() async throws { called = true }
    }

    // MARK: - サインアウト

    func testSignOutRunsTheSameWipeAsAccountDeletionAndKeepsPreservedSettings() async throws {
        let d = UserDefaults.standard
        let keep = Array(LocalWipe.preservedKeys)
        let saved = keep.map { ($0, d.object(forKey: $0)) }
        defer {
            for (key, value) in saved {
                if let value { d.set(value, forKey: key) } else { d.removeObject(forKey: key) }
            }
        }
        d.set(AppLanguage.ja.rawValue, forKey: "metamon.appLanguage")
        d.set(false, forKey: "metamon.crashReporting.enabled")
        d.set(false, forKey: "metamon.aiButtonMetrics.enabled")
        d.set(true, forKey: TimerKeyMigration.doneKey)
        let keptValues = keep.map { d.object(forKey: $0) as? NSObject }
        d.set("前の人の状態", forKey: "metamon.signOutTest.userState")
        d.set(Data([1]), forKey: "metamon.timer.prev-app/main/t1") // タイマーの走行状態

        let services = BuildConfig.make(environment: .mock)
        let rt = AppRuntime(document: try DSLExamples.document("kakeibo", version: 1), services: services)
        rt.apps = [AppSummary(id: "prev-app", name: "前の人のアプリ", currentVersion: 1)]
        let oldAppId = rt.document.appId
        let session = AuthSession(service: services.auth)
        XCTAssertNotNil(session.user)

        let outcome = try await session.signOutAndWipe(runtime: rt, choice: .sendThenWipe)

        XCTAssertEqual(outcome, .signedOut)
        XCTAssertNil(session.user)
        XCTAssertTrue(session.farewell)
        XCTAssertTrue(rt.apps.isEmpty, "前の人のアプリ一覧を残さない")
        XCTAssertNotEqual(rt.document.appId, oldAppId, "次の人は新しい appId から")
        XCTAssertTrue(rt.dsl.isVoiceOnly)
        XCTAssertEqual(rt.recordSync.pendingCount, 0)
        XCTAssertNil(d.object(forKey: "metamon.signOutTest.userState"))
        XCTAssertNil(d.object(forKey: "metamon.timer.prev-app/main/t1"))
        XCTAssertEqual(keep.map { d.object(forKey: $0) as? NSObject }, keptValues, "言語・送信のオプトアウト・移行の印は残す")
    }

    #if !APP_STORE_REVIEW
    /// #1318: feed の覚えた行(前の人の問い合わせの結果)も消す。
    func testWipeClearsRememberedFeedRows() async throws {
        struct OneRow: FeedClient {
            func feed(_ query: FeedQuery) async throws -> FeedResult {
                FeedResult(provider: query.provider, title: "前の人の検索", items: [], source: FeedSource(name: "n", url: "https://x", license: "l"))
            }
        }
        let q = FeedQuery(provider: "diet", q: "前の人の検索語")
        defer { FeedCache.shared.reset() }
        _ = try await FeedCache.shared.load(q, force: false, client: OneRow())
        XCTAssertNotNil(FeedCache.shared.cached(q))

        let rt = AppRuntime(document: try DSLExamples.document("kakeibo", version: 1), services: BuildConfig.make(environment: .mock))
        await rt.wipeLocalData()

        XCTAssertNil(FeedCache.shared.cached(q), "前の人の feed の行を残さない")
    }
    #endif

    func testPreservedKeysAreTheThreeSettingsPlusDeviceMarks() {
        XCTAssertEqual(LocalWipe.preservedKeys, [
            "metamon.appLanguage",
            "metamon.crashReporting.enabled",
            "metamon.aiButtonMetrics.enabled",
            PushDeviceRegistrar.installationIdKey,
            TimerKeyMigration.doneKey,
        ])
    }

    func testTimerMigrationMarkSurvivesWipeSoMigrationDoesNotRunAgain() {
        let suite = "signout-\(UUID().uuidString)"
        let defaults = UserDefaults(suiteName: suite)!
        defer { defaults.removePersistentDomain(forName: suite) }
        defaults.set(true, forKey: TimerKeyMigration.doneKey)
        defaults.set(Data([1]), forKey: "metamon.timer.家計簿/main/t1") // 古い形のキー

        LocalWipe.clearDefaults(defaults, domain: suite)

        XCTAssertTrue(defaults.bool(forKey: TimerKeyMigration.doneKey))
        XCTAssertNil(defaults.object(forKey: "metamon.timer.家計簿/main/t1"))
    }

    func testSignOutMessages() {
        let anon = SignOutCopy.message(isAnonymous: true, prompt: .plain)
        XCTAssertTrue(anon.contains("匿名のアカウントには戻れません"), "匿名は戻れないことをはっきり出す")
        XCTAssertTrue(anon.contains("この端末のデータが消え"))
        let apple = SignOutCopy.message(isAnonymous: false, prompt: .plain)
        XCTAssertFalse(apple.contains("匿名"))
        XCTAssertTrue(apple.contains("この端末のデータは消えます"))
        XCTAssertTrue(SignOutCopy.message(isAnonymous: false, prompt: .unsent(count: 2, canSend: true)).contains("送ってからログアウトします"))
        XCTAssertTrue(SignOutCopy.message(isAnonymous: false, prompt: .unsent(count: 2, canSend: false)).contains("オフライン"))
    }

    // MARK: - 生成中は削除できない

    func testDeletingAppsAndAccountIsBlockedWhileGenerating() async throws {
        let ai = UndoRedoTests.GatedAI()
        var services = BuildConfig.make(environment: .mock)
        services.ai = ai
        let rt = AppRuntime(document: try DSLExamples.document("kakeibo", version: 1), services: services)
        rt.clarifyEnabled = false
        XCTAssertTrue(rt.canDelete)

        ai.holdNextMorph()
        let job = Task { await rt.generate("金額を大きくして") }
        var started = ai.started.makeAsyncIterator()
        _ = await started.next()
        XCTAssertTrue(rt.isMorphing)
        XCTAssertFalse(rt.canDelete, "ボタンを押せない状態にする")

        // 確認を出した後に生成が始まった時: 呼ばれても何もしない
        let other = AppSummary(id: "other", name: "別アプリ", currentVersion: 1)
        rt.apps = [other]
        await rt.deleteApp(other)
        XCTAssertEqual(rt.apps.map(\.id), ["other"], "アプリを消さない")
        XCTAssertEqual(rt.toast, AppRuntime.deleteBlockedReason)

        let account = RecordingAccount()
        let session = AuthSession(service: services.auth)
        let deleted = try await session.deleteAccount(account: account, runtime: rt)
        XCTAssertFalse(deleted)
        XCTAssertFalse(account.called, "サーバーにも送らない")
        XCTAssertNotNil(session.user)

        ai.releaseMorph()
        await job.value
        XCTAssertTrue(rt.canDelete, "生成が終われば削除できる")
    }
}
