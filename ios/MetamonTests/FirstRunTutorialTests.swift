import XCTest
import MetamonDSL
import MetamonServices
@testable import Metamon

/// #274 初回のチュートリアル・カテゴリ別の例・直し方の例 / #275 音声モデルの準備中も文字で始められる案内。
@MainActor
final class FirstRunTutorialTests: XCTestCase {
    private var defaults: UserDefaults!
    private var suite = ""

    override func setUp() {
        suite = "tutorial274-\(UUID().uuidString)"
        defaults = UserDefaults(suiteName: suite)
    }

    override func tearDown() {
        defaults.removePersistentDomain(forName: suite)
        defaults = nil
    }

    private func makeRuntime() throws -> AppRuntime {
        let rt = AppRuntime(document: try DSLExamples.document(DSLExamples.initial, version: 0),
                            services: BuildConfig.make(environment: .mock))
        rt.aiConsent = AIConsentStore(defaults: defaults)
        rt.tutorial = FirstRunTutorialStore(defaults: defaults)
        return rt
    }

    // MARK: - 保存

    func testStorePersistsSeenAndEditHint() {
        let store = FirstRunTutorialStore(defaults: defaults)
        XCTAssertFalse(store.hasSeenTutorial)
        XCTAssertTrue(store.shouldAutoPresent(consentPrompted: true))
        XCTAssertFalse(store.shouldAutoPresent(consentPrompted: false), "同意の説明より先に出してはいけない")
        store.markTutorialSeen()
        XCTAssertFalse(store.shouldAutoPresent(consentPrompted: true))

        XCTAssertTrue(store.shouldShowEditHint(createdFromInitial: true))
        XCTAssertFalse(store.shouldShowEditHint(createdFromInitial: false), "既存アプリの修正では出さない")
        store.markEditHintShown()
        XCTAssertFalse(store.shouldShowEditHint(createdFromInitial: true), "1 回だけ")

        let reloaded = FirstRunTutorialStore(defaults: defaults)
        XCTAssertTrue(reloaded.hasSeenTutorial)
        XCTAssertTrue(reloaded.hasShownEditHint)
    }

    func testLaunchOverride() {
        let store = FirstRunTutorialStore(defaults: defaults)
        store.applyLaunchOverride(environment: ["METAMON_AI_CONSENT": "granted"])
        XCTAssertTrue(store.hasSeenTutorial, "既存の自動テスト(同意済み起動)はチュートリアルで塞がない")
        store.applyLaunchOverride(environment: ["METAMON_AI_CONSENT": "granted", "METAMON_TUTORIAL": "reset"])
        XCTAssertFalse(store.hasSeenTutorial)
        store.applyLaunchOverride(environment: [:])
        XCTAssertFalse(store.hasSeenTutorial, "指定なしでは何もしない")
        store.applyLaunchOverride(environment: ["METAMON_TUTORIAL": "seen"])
        XCTAssertTrue(store.hasSeenTutorial)
        XCTAssertTrue(store.hasShownEditHint)
    }

    // MARK: - 順序: 同意 → チュートリアル → 音声モデルの確認

    func testTutorialWaitsForConsentThenGatesModelDownloadConfirm() throws {
        let rt = try makeRuntime()
        rt.showModelDownloadConfirm = true

        rt.presentTutorialIfNeeded()
        XCTAssertFalse(rt.showTutorial, "同意の説明を出す前はチュートリアルを出さない")
        XCTAssertFalse(rt.canShowModelDownloadConfirm, "チュートリアル前にダウンロード確認を出さない")

        rt.showAIConsent = true
        rt.aiConsent?.markPrompted()
        rt.presentTutorialIfNeeded()
        XCTAssertFalse(rt.showTutorial, "同意の画面が出ている間は出さない")

        rt.showAIConsent = false
        rt.presentTutorialIfNeeded()
        XCTAssertTrue(rt.showTutorial)
        XCTAssertFalse(rt.canShowModelDownloadConfirm)

        rt.finishTutorial()
        XCTAssertFalse(rt.showTutorial)
        XCTAssertTrue(rt.tutorial?.hasSeenTutorial == true)
        XCTAssertTrue(rt.canShowModelDownloadConfirm, "チュートリアルのあとにダウンロード確認")

        rt.presentTutorialIfNeeded()
        XCTAssertFalse(rt.showTutorial, "見たら次からは自動で出さない")
    }

    func testRuntimeWithoutStoreTreatsTutorialAsSeen() throws {
        let rt = AppRuntime(document: try DSLExamples.document(DSLExamples.initial, version: 0),
                            services: BuildConfig.make(environment: .mock))
        rt.presentTutorialIfNeeded()
        XCTAssertFalse(rt.showTutorial)
        rt.showModelDownloadConfirm = true
        XCTAssertTrue(rt.canShowModelDownloadConfirm)
    }

    // MARK: - 最初のアプリの直後の「こう話すと直せます」

    func testEditHintIsConsumedOnFirstCreateOnly() async throws {
        let rt = try makeRuntime()
        rt.aiConsent?.grant()
        await rt.generate("家計簿アプリを作って")
        XCTAssertFalse(rt.dsl.isVoiceOnly, "モックで生成できる")
        XCTAssertTrue(rt.tutorial?.hasShownEditHint == true, "初期画面からの作成で直し方の例を予約する")

        let other = try makeRuntime()
        other.tutorial?.markEditHintShown()
        other.scheduleFirstEditHint(createdFromInitial: true)
        XCTAssertFalse(other.showFirstEditHint)
    }

    // MARK: - カードと例(審査用の版ではシェアを案内しない)

    func testCardsDropSharingWhenUnavailable() {
        let full = FirstRunTutorial.cards(sharingAvailable: true, voiceModelMegabytes: 500)
        XCTAssertEqual(full.map(\.id), ["create", "edit", "share"])
        XCTAssertNotNil(full[0].note, "1 枚目に文字ですぐ作れる案内(#275)")
        XCTAssertTrue(full[0].note?.contains("500") == true)

        let review = FirstRunTutorial.cards(sharingAvailable: false, voiceModelMegabytes: 500)
        XCTAssertEqual(review.map(\.id), ["create", "edit", "list"])
    }

    func testExampleCategoriesIncludeSharedExamples() {
        let full = Greeting.createExampleCategories(sharingAvailable: true)
        XCTAssertEqual(full.map(\.id), ["record", "habit", "money", "together", "game"])
        XCTAssertTrue(full.allSatisfy { !$0.examples.isEmpty })
        XCTAssertEqual(full.first { $0.id == "together" }?.examples.count, 3, "割り勘・出欠・みんなの掲示板")

        let review = Greeting.createExampleCategories(sharingAvailable: false)
        XCTAssertEqual(review.first { $0.id == "together" }?.examples.count, 2, "審査用の版は掲示板を出さない")
        XCTAssertFalse(Greeting.createExamples.isEmpty)
    }

    // MARK: - #275 音声モデル: モバイル回線・空き容量

    func testModelDownloadAsksAgainOnExpensiveNetwork() {
        ModelDownloadConsent.approve(defaults)
        XCTAssertEqual(ModelDownloadConsent.decide(needsDownload: true, isExpensiveNetwork: false, defaults: defaults), .prepare)
        XCTAssertEqual(ModelDownloadConsent.decide(needsDownload: true, isExpensiveNetwork: true, defaults: defaults), .ask,
                       "モバイル回線では承認済みでも聞き直す")
        XCTAssertEqual(ModelDownloadConsent.decide(needsDownload: false, isExpensiveNetwork: true, defaults: defaults), .prepare)
    }

    func testModelDownloadAdvice() {
        let ok = ModelDownloadAdvice.evaluate(megabytes: 500, isExpensiveNetwork: false, availableBytes: 5_000_000_000)
        XCTAssertEqual(ok, ModelDownloadAdvice(isExpensiveNetwork: false, shortageMegabytes: nil))

        let low = ModelDownloadAdvice.evaluate(megabytes: 500, isExpensiveNetwork: true, availableBytes: 400_000_000)
        XCTAssertTrue(low.isExpensiveNetwork)
        XCTAssertEqual(low.shortageMegabytes, 600)

        XCTAssertNil(ModelDownloadAdvice.evaluate(megabytes: 500, isExpensiveNetwork: false, availableBytes: nil).shortageMegabytes,
                     "容量が取れない時は注意を出さない")

        let message = ConsentHost.downloadMessage(advice: low)
        XCTAssertTrue(message.contains("600"))
        XCTAssertEqual(message.components(separatedBy: "\n\n").count, 3, "回線・容量・本文")
        XCTAssertEqual(ConsentHost.downloadMessage(advice: ok).components(separatedBy: "\n\n").count, 1)
    }
}
