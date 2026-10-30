import XCTest
import MetamonDSL
import MetamonServices
import MetamonRenderer
@testable import Metamon

/// #414: 有料機能(共有の記録)を使う全経路で、Pro でなければ課金画面を通る。AppRuntime + モック AI + モックのストアで確かめる。
/// モック ai の「掲示板」は誰でも見られる(scope public)ひとこと投稿を返す。無料版・「自分だけ」の指示なら家計簿(private)。
@MainActor
final class PaidGateFlowTests: XCTestCase {
    private let sharedRequest = "サークルのみんなで予定を書き込める掲示板アプリを作って"

    private func makeRuntime(enforced: Bool = true, clarify: Bool, example: String = DSLExamples.initial) throws -> (AppRuntime, PaidFeatureConfirmTests.SpyAI) {
        let spy = PaidFeatureConfirmTests.SpyAI()
        var services = BuildConfig.make(environment: .mock)
        services.ai = spy
        let version = example == DSLExamples.initial ? 0 : 1
        let rt = AppRuntime(document: try DSLExamples.document(example, version: version), services: services)
        rt.clarifyEnabled = clarify
        rt.clarifyAutoBuild = false
        let defaults = UserDefaults(suiteName: "PaidGateFlowTests.\(UUID().uuidString)")!
        rt.proGate = ProGate(store: ProEntitlementStore(mode: .mock), usage: CreationUsageStore(defaults: defaults),
                             quota: CreationQuota(dailyFreeLimit: 100), limitDisabled: !enforced)
        return (rt, spy)
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

    /// 共有を外す修正の指示(表示言語に合わせる・テストのシミュレータは英語のこともある)
    private var dropInstruction: String { PaidGate.dropSharedDataInstruction(english: LanguagePreference.shared.code == "en") }

    private func buy(_ rt: AppRuntime) async throws {
        let outcome = try await rt.proGate.store.purchaseOffer()
        XCTAssertEqual(outcome, .purchased)
        rt.proGate.finishPaywall() // 購入後に課金画面が自動で閉じる(PaywallSheet の onChange → dismiss → onDismiss)
    }

    // MARK: - 判定(DSL)

    func testIntroducesSharedDataFromDSL() throws {
        let initial = try DSLExamples.load("initial")
        let kakeibo = try DSLExamples.load("kakeibo")
        let board = try DSLExamples.load("post_comments")
        XCTAssertFalse(PaidGate.usesSharedData(kakeibo))
        XCTAssertTrue(PaidGate.usesSharedData(board))
        XCTAssertTrue(PaidGate.introducesSharedData(from: initial, to: board), "新規作成で共有")
        XCTAssertTrue(PaidGate.introducesSharedData(from: kakeibo, to: board), "修正で共有を足す")
        XCTAssertFalse(PaidGate.introducesSharedData(from: board, to: board), "もとから共有のアプリの修正は聞き直さない")
        XCTAssertFalse(PaidGate.introducesSharedData(from: board, to: kakeibo), "共有を外す修正")
        XCTAssertTrue(PaidGate.generatedNeedsPaywall(before: kakeibo, after: board, isPro: false, enforced: true))
        XCTAssertFalse(PaidGate.generatedNeedsPaywall(before: kakeibo, after: board, isPro: true, enforced: true))
        XCTAssertFalse(PaidGate.generatedNeedsPaywall(before: kakeibo, after: board, isPro: false, enforced: false))
    }

    // MARK: - チャットの「これでいく」

    /// 確認カードと一緒に課金画面(chat)→ 購入 → 有料機能込みの文で生成 → 共有の掲示板
    func testChatPaywallPurchaseBuildsWithPaidFeature() async throws {
        let (rt, spy) = try makeRuntime(clarify: true)
        await rt.submitUtterance(sharedRequest)
        await rt.buildFromChat()
        XCTAssertNotNil(rt.paidConfirm)
        let request = try await waitForPaywall(rt)
        XCTAssertEqual(request.placement, .chat)
        XCTAssertEqual(request.reason, .paidFeature([MockAIClient.sharedDataFeature]))
        XCTAssertTrue(spy.sent.isEmpty, "課金画面の間は生成しない")

        try await buy(rt)
        try await waitUntil { !rt.dsl.isVoiceOnly }
        XCTAssertEqual(spy.sent, [sharedRequest], "有料機能込みの文で作る")
        XCTAssertTrue(PaidGate.usesSharedData(rt.dsl))
        XCTAssertNil(rt.proGate.paywall, "購入後は生成の後に聞き直さない")
    }

    /// 課金画面を閉じる → 何も作らずカードに残る(「この機能なしで作る」/「やめる」を選べる)
    func testChatPaywallDismissKeepsCardAndBuildsNothing() async throws {
        let (rt, spy) = try makeRuntime(clarify: true)
        await rt.submitUtterance(sharedRequest)
        await rt.buildFromChat()
        _ = try await waitForPaywall(rt)
        rt.proGate.finishPaywall()
        try await Task.sleep(for: .milliseconds(200))
        XCTAssertNotNil(rt.paidConfirm, "カードに戻る")
        XCTAssertFalse(rt.paidConfirmBusy)
        XCTAssertTrue(spy.sent.isEmpty, "閉じただけでは作らない")

        // カードの「購入して作る」でもう一度課金画面
        let again = Task { await rt.purchaseAndBuild() }
        _ = try await waitForPaywall(rt)
        rt.dismissPaidConfirm() // やめる = 課金画面ごと閉じる
        await again.value
        XCTAssertNil(rt.paidConfirm)
        XCTAssertNil(rt.proGate.paywall)
        XCTAssertTrue(spy.sent.isEmpty)
    }

    // MARK: - チャットを通らない経路(生成の後に判定)

    /// チャット OFF の即生成で共有ができた → 課金画面(root)→ 閉じる → 共有を外す修正が走り、自分だけの記録になる
    func testDirectGenerationDismissDropsSharedData() async throws {
        let (rt, spy) = try makeRuntime(clarify: false)
        let run = Task { await rt.submitUtterance("みんなの掲示板アプリを作って") }
        let request = try await waitForPaywall(rt)
        XCTAssertEqual(request.placement, .root)
        XCTAssertNotNil(request.closeNote)
        XCTAssertTrue(PaidGate.usesSharedData(rt.dsl), "生成の結果(共有)の上に課金画面")

        rt.proGate.finishPaywall()
        await run.value
        XCTAssertEqual(spy.sent.count, 2)
        XCTAssertEqual(spy.sent.last, dropInstruction)
        XCTAssertFalse(PaidGate.usesSharedData(rt.dsl), "買わなければ共有は残らない")
        XCTAssertFalse(rt.dsl.isVoiceOnly, "アプリは残る(共有なし)")
        XCTAssertEqual(rt.proGate.usedToday, 1, "新規作成は 1 回だけ数える")
    }

    /// 即生成 → 課金画面で購入 → 共有のまま
    func testDirectGenerationPurchaseKeepsSharedData() async throws {
        let (rt, spy) = try makeRuntime(clarify: false)
        let run = Task { await rt.submitUtterance("みんなの掲示板アプリを作って") }
        _ = try await waitForPaywall(rt)
        try await buy(rt)
        await run.value
        XCTAssertEqual(spy.sent.count, 1)
        XCTAssertTrue(PaidGate.usesSharedData(rt.dsl))
    }

    /// 作ったアプリの修正(チャット OFF)で共有を足す → 課金画面 → 閉じると共有を外す
    func testEditAddingSharedDataGoesThroughPaywall() async throws {
        let (rt, spy) = try makeRuntime(clarify: false, example: "kakeibo")
        let run = Task { await rt.submitUtterance("みんなで書き込める掲示板にして") }
        _ = try await waitForPaywall(rt)
        rt.proGate.finishPaywall()
        await run.value
        XCTAssertEqual(spy.sent.last, dropInstruction)
        XCTAssertFalse(PaidGate.usesSharedData(rt.dsl))
    }

    /// 「元に戻す」で共有の版に戻った時も課金画面。閉じると戻す前の版へ
    func testUndoToSharedVersionGoesThroughPaywall() async throws {
        let (rt, _) = try makeRuntime(enforced: false, clarify: false, example: "kakeibo")
        await rt.submitUtterance("みんなの掲示板にして") // 制限の無効中に共有にした
        XCTAssertTrue(PaidGate.usesSharedData(rt.dsl))
        await rt.submitUtterance("家計簿の記録アプリにして")
        XCTAssertFalse(PaidGate.usesSharedData(rt.dsl))

        // 課金の制限を有効にする(limitDisabled の書き換えは端末の設定に残るので、ProGate ごと差し替える)
        rt.proGate = ProGate(store: rt.proGate.store, usage: rt.proGate.usage, quota: rt.proGate.quota, limitDisabled: false)
        let undo = Task { await rt.undo() }
        _ = try await waitForPaywall(rt)
        XCTAssertTrue(PaidGate.usesSharedData(rt.dsl))
        rt.proGate.finishPaywall()
        await undo.value
        XCTAssertFalse(PaidGate.usesSharedData(rt.dsl), "閉じたら戻す前の版")
    }

    /// デモ端末(課金の制限が無効)は課金画面を出さずに共有で作る
    func testNotEnforcedNeverShowsPaywall() async throws {
        let (rt, spy) = try makeRuntime(enforced: false, clarify: false)
        await rt.submitUtterance("みんなの掲示板アプリを作って")
        XCTAssertNil(rt.proGate.paywall)
        XCTAssertEqual(spy.sent.count, 1)
        XCTAssertTrue(PaidGate.usesSharedData(rt.dsl))
    }

    /// 共有の無いアプリは課金画面を出さない
    func testPrivateAppNeverShowsPaywall() async throws {
        let (rt, _) = try makeRuntime(clarify: false)
        await rt.submitUtterance("毎日の支出を記録する家計簿アプリを作って")
        XCTAssertNil(rt.proGate.paywall)
        XCTAssertFalse(rt.dsl.isVoiceOnly)
    }

    // MARK: - ProGate

    func testRequestProReturnsImmediatelyForPro() async throws {
        let (rt, _) = try makeRuntime(clarify: false)
        _ = try await rt.proGate.store.purchaseOffer()
        let pro = await rt.proGate.requestPro(.join(appName: "家族の買い物"))
        XCTAssertTrue(pro)
        XCTAssertNil(rt.proGate.paywall)
    }

    func testRequestProWaitsUntilClosed() async throws {
        let (rt, _) = try makeRuntime(clarify: false)
        let wait = Task { await rt.proGate.requestPro(.join(appName: "家族の買い物")) }
        let request = try await waitForPaywall(rt)
        XCTAssertEqual(request.reason, .join(appName: "家族の買い物"))
        rt.proGate.dismissPaywall(placement: .chat)
        XCTAssertNotNil(rt.proGate.paywall, "別の場所の課金画面は閉じない")
        rt.proGate.dismissPaywall(placement: .root)
        let pro = await wait.value
        XCTAssertFalse(pro)
    }

    private func waitUntil(timeout: Duration = .seconds(10), _ condition: () -> Bool) async throws {
        let deadline = ContinuousClock.now + timeout
        while !condition() {
            guard ContinuousClock.now < deadline else { return XCTFail("時間内に条件が成り立たない") }
            try await Task.sleep(for: .milliseconds(20))
        }
    }
}
