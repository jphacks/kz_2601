import XCTest
import MetamonDSL
import MetamonServices
@testable import Metamon

/// UX の通し点検(docs/ux/ux-audit-2026-11.md)で 2026-11-06 にオーナーが決めた分。
/// 課金画面の出し方(H4・H5)は `PaidGateFlowTests`・`ShareJoinFlowTests`、β の案内は `BetaWelcomeTests`。
@MainActor
final class UXDecidedTests: XCTestCase {
    private var defaults: UserDefaults!
    private var suite = ""

    override func setUp() {
        suite = "uxDecided-\(UUID().uuidString)"
        defaults = UserDefaults(suiteName: suite)
    }

    override func tearDown() {
        defaults.removePersistentDomain(forName: suite)
        defaults = nil
    }

    /// 端末に音声モデルが無い文字起こし(ダウンロード前)。
    private final class NotDownloadedTranscriber: SpeechTranscriber {
        var isReady: Bool { false }
        var status: TranscriberStatus { .notDownloaded }
        var needsDownload: Bool { true }
        private(set) var prepared = false
        func prepare() async throws { prepared = true }
        func startRecording() async throws {}
        func stopAndTranscribe() async throws -> String { "" }
    }

    private func makeRuntime(transcriber: (any SpeechTranscriber)? = nil, ai: (any AIClient)? = nil) throws -> AppRuntime {
        var services = BuildConfig.make(environment: .mock)
        if let transcriber { services.transcriber = transcriber }
        if let ai { services.ai = ai }
        let rt = AppRuntime(document: try DSLExamples.document(DSLExamples.initial, version: 0), services: services)
        rt.aiConsent = AIConsentStore(defaults: defaults)
        rt.tutorial = FirstRunTutorialStore(defaults: defaults)
        rt.betaWelcome = BetaWelcomeStore(defaults: defaults, isBeta: true)
        rt.clarifyEnabled = false
        rt.proGate = ProGate(store: ProEntitlementStore(mode: .mock), usage: CreationUsageStore(defaults: defaults),
                             quota: CreationQuota(dailyFreeLimit: 100), limitDisabled: true)
        return rt
    }

    // MARK: - H1・M14 初回の画面の数(U6)

    /// 最初の入力までに出る画面: 同意(審査 5.1.2 のため残す)とチュートリアル 1 枚の 2 つだけ。
    /// 前は 同意 → チュートリアル 3 枚 → β の案内 → 音声モデルの確認 の 4 つ(タップは最短 5)。
    func testFirstRunShowsOnlyConsentAndOneTutorialCard() throws {
        let transcriber = NotDownloadedTranscriber()
        let rt = try makeRuntime(transcriber: transcriber)
        var screens: [String] = []

        // 起動: 音声モデルが無くても確認のアラートは自動で出さない(M14)
        rt.prepareTranscriberIfAllowed(defaults: defaults, isExpensiveNetwork: false)
        XCTAssertFalse(rt.showModelDownloadConfirm, "起動で音声モデルの確認を出さない")
        XCTAssertFalse(transcriber.prepared, "承認前にダウンロードを始めない(4.2.3(ii))")

        // 1. 同意(`ConsentHost` が未同意・未提示の時に出す)
        XCTAssertFalse(rt.hasAIConsent)
        screens.append("consent")
        rt.aiConsent?.markPrompted()
        rt.grantAIConsent()

        // 2. チュートリアル(1 枚)
        rt.presentTutorialIfNeeded()
        XCTAssertTrue(rt.showTutorial)
        screens.append(contentsOf: FirstRunTutorial.cards(voiceModelMegabytes: 500).map { "tutorial.\($0.id)" })
        rt.finishTutorial()

        // β の案内は最初のアプリの後(初回の関門には入らない)
        rt.presentBetaWelcomeIfNeeded()
        XCTAssertFalse(rt.showBetaWelcome, "最初のアプリの前に β の案内を出さない")
        XCTAssertFalse(rt.canShowModelDownloadConfirm, "求められていない確認は出さない")

        XCTAssertEqual(screens, ["consent", "tutorial.create"], "最初の入力までの画面は 2 つ")
    }

    /// M14: 音声モデルの確認は、初めてマイクを押した時に出す(録音は始めない)。承認済みなら聞かない
    func testMicAsksModelDownloadOnlyWhenNotApproved() throws {
        let rt = try makeRuntime(transcriber: NotDownloadedTranscriber())
        XCTAssertTrue(rt.askModelDownloadBeforeRecording(defaults: defaults, isExpensiveNetwork: false), "未承認ならマイクで確認を出す")
        XCTAssertTrue(rt.showModelDownloadConfirm)

        rt.showModelDownloadConfirm = false
        ModelDownloadConsent.approve(defaults)
        XCTAssertFalse(rt.askModelDownloadBeforeRecording(defaults: defaults, isExpensiveNetwork: false), "承認済み(ダウンロード中)は聞かない")
        XCTAssertFalse(rt.showModelDownloadConfirm)

        let ready = try makeRuntime()
        XCTAssertFalse(ready.askModelDownloadBeforeRecording(defaults: defaults, isExpensiveNetwork: false), "モデルがあれば聞かない")
    }

    /// #275: 従量回線(テザリング・省データモード)では承認済みでも聞き直す。モデルがあれば回線に関係なく聞かない
    func testMicAsksModelDownloadOnExpensiveNetworkEvenIfApproved() throws {
        let rt = try makeRuntime(transcriber: NotDownloadedTranscriber())
        ModelDownloadConsent.approve(defaults)
        XCTAssertTrue(rt.askModelDownloadBeforeRecording(defaults: defaults, isExpensiveNetwork: true), "従量回線なら承認済みでも確認を出す")
        XCTAssertTrue(rt.showModelDownloadConfirm)

        let ready = try makeRuntime()
        XCTAssertFalse(ready.askModelDownloadBeforeRecording(defaults: defaults, isExpensiveNetwork: true), "モデルがあれば聞かない")
    }

    // MARK: - M2 同意の前に話した内容を捨てない

    func testUtteranceBeforeConsentIsSentAfterGranting() async throws {
        let spy = PaidFeatureConfirmTests.SpyAI()
        let rt = try makeRuntime(ai: spy)
        await rt.submitUtterance("家計簿アプリを作って")
        XCTAssertTrue(rt.showAIConsent, "未同意なら同意の画面")
        XCTAssertTrue(spy.sent.isEmpty, "同意の前は送らない")

        rt.grantAIConsent()
        await rt.resumeUtteranceAfterConsent()
        XCTAssertEqual(spy.sent, ["家計簿アプリを作って"], "同意したら話し直さずに送る")
        XCTAssertFalse(rt.dsl.isVoiceOnly)
        XCTAssertNil(rt.utteranceAwaitingConsent)
    }

    func testUtteranceIsDroppedWhenConsentIsPostponed() async throws {
        let spy = PaidFeatureConfirmTests.SpyAI()
        let rt = try makeRuntime(ai: spy)
        await rt.submitUtterance("家計簿アプリを作って")
        rt.aiConsent?.markPrompted() // 「あとで」
        rt.showAIConsent = false
        await rt.resumeUtteranceAfterConsent()
        XCTAssertNil(rt.utteranceAwaitingConsent, "「あとで」なら捨てる")

        rt.grantAIConsent() // 後で設定から同意
        await rt.resumeUtteranceAfterConsent()
        XCTAssertTrue(spy.sent.isEmpty, "古い発話を勝手に送らない")
    }

    // MARK: - H10 一覧・設定の入口(U1)

    func testAppListButtonOnVoiceTopBar() {
        XCTAssertFalse(VoiceTopBar.showsAppList(appCount: 0, isVoiceOnly: true, madeFirstApp: false), "初回の初期画面は空の一覧しか開けないので出さない")
        XCTAssertTrue(VoiceTopBar.showsAppList(appCount: 0, isVoiceOnly: false, madeFirstApp: false), "アプリを開いている時(音声シート)は出す")
        XCTAssertTrue(VoiceTopBar.showsAppList(appCount: 0, isVoiceOnly: true, madeFirstApp: true), "前に作った人の初期画面にも出す")
        XCTAssertTrue(VoiceTopBar.showsAppList(appCount: 2, isVoiceOnly: true, madeFirstApp: false))
    }

    /// 変身一覧の歯車から開いた設定で「使い方を見る」→ 一覧を閉じてからチュートリアル
    func testReplayTutorialClosesSwitcher() throws {
        let rt = try makeRuntime()
        rt.showAppSwitcher = true
        rt.replayTutorial()
        XCTAssertFalse(rt.showAppSwitcher, "一覧の上にチュートリアル(全画面)は出せない")
    }

    // MARK: - M5 答えのチップを折り返す

    func testChipsWrapToNextRow() {
        let chip = CGSize(width: 120, height: 36)
        let rows = ChipFlowLayout.rows(sizes: [chip, chip, chip], maxWidth: 260, spacing: 8)
        XCTAssertEqual(rows.map(\.items), [[0, 1], [2]], "入らないチップは次の行へ(横に隠さない)")
        XCTAssertEqual(rows[1].y, 44)
        XCTAssertEqual(ChipFlowLayout.rows(sizes: [chip, chip], maxWidth: 400, spacing: 8).count, 1, "入れば 1 行")
        let long = ChipFlowLayout.rows(sizes: [CGSize(width: 500, height: 60), chip], maxWidth: 300, spacing: 8)
        XCTAssertEqual(long.map(\.items), [[0], [1]], "幅を超えるチップは 1 行に 1 つ")
        XCTAssertEqual(long[0].width, 300)
    }
}
