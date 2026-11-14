import XCTest
import MetamonDSL
import MetamonServices
@testable import Metamon

/// H7: 完成の直後のお知らせは重ねず、課金 → 要約 → 公開の確認 → 保存期間 → 直し方 → 引き継ぎ の順に 1 つずつ出す。
@MainActor
final class PostBuildNoticeTests: XCTestCase {
    private var defaults: UserDefaults!
    private var suite = ""

    override func setUp() {
        suite = "PostBuildNoticeTests.\(UUID().uuidString)"
        defaults = UserDefaults(suiteName: suite)
    }

    override func tearDown() {
        defaults.removePersistentDomain(forName: suite)
        defaults = nil
    }

    func testOrderIsOneAtATime() {
        var pending = Set(PostBuildNotice.allCases)
        var shown: [PostBuildNotice] = []
        while let next = PostBuildNotice.current(pending) {
            shown.append(next)
            pending.remove(next) // 閉じたら次
        }
        XCTAssertEqual(shown, [.paywall, .summary, .publicData, .retention, .editHint, .accountLink])
        XCTAssertNil(PostBuildNotice.current([]))
    }

    func testRuntimeGatesEachNoticeBehindTheEarlierOnes() throws {
        let rt = AppRuntime(document: try DSLExamples.document("kakeibo", version: 1), services: BuildConfig.make(environment: .mock))
        rt.showLinkPrompt = true
        rt.showFirstEditHint = true
        rt.retentionPrompt = RetentionPrompt(appId: rt.document.appId, version: 1,
                                             entities: [try JSONDecoder().decode(Entity.self, from: Data(#"{"id":"diary","name":"日記","retentionDays":3,"fields":[]}"#.utf8))])
        rt.summaryBanner = AppRuntime.SummaryBanner(text: "できました", highlight: nil, action: .undo)
        rt.paidGatePending = true

        XCTAssertEqual(rt.activeNotice, .paywall, "共有の版の課金画面が最初")
        XCTAssertFalse(rt.isNoticeTurn(.summary), "要約は課金画面が閉じてから(下で時間切れにしない)")
        rt.paidGatePending = false
        XCTAssertEqual(rt.activeNotice, .summary)
        rt.summaryBanner = nil
        XCTAssertEqual(rt.activeNotice, .retention)
        rt.deferRetention()
        XCTAssertEqual(rt.activeNotice, .editHint)
        rt.showFirstEditHint = false
        XCTAssertEqual(rt.activeNotice, .accountLink, "引き継ぎの案内は最後")
        rt.showLinkPrompt = false
        XCTAssertNil(rt.activeNotice)
    }

    /// 最初のアプリの完成: 要約 → 公開の確認 → 直し方 の順。引き継ぎの案内は出さない(次の起動へ)
    func testFirstAppQueuesNoticesInOrder() async throws {
        var services = BuildConfig.make(environment: .mock)
        services.ai = MockAIClient(delay: .milliseconds(10))
        let rt = AppRuntime(document: try DSLExamples.document(DSLExamples.initial, version: 0), services: services)
        rt.clarifyEnabled = false
        rt.aiConsent = AIConsentStore(defaults: defaults)
        rt.aiConsent?.grant()
        rt.tutorial = FirstRunTutorialStore(defaults: defaults)
        rt.proGate = ProGate(store: ProEntitlementStore(mode: .mock), usage: CreationUsageStore(defaults: defaults),
                             quota: CreationQuota(dailyFreeLimit: 100), limitDisabled: true)

        await rt.submitUtterance("美容室の予約アプリを作って")
        XCTAssertFalse(rt.dsl.isVoiceOnly, "モックで生成できる")
        XCTAssertNotNil(rt.summaryBanner)
        XCTAssertNotNil(rt.publicDataPrompt, "モックの予約は公開の記録に個人の情報を返す")
        XCTAssertEqual(rt.activeNotice, .summary, "まず要約(「元に戻す」付き)")
        XCTAssertFalse(rt.showLinkPrompt, "最初のアプリの直後に引き継ぎの案内を出さない")

        let deadline = ContinuousClock.now + .seconds(3)
        while !rt.showFirstEditHint, ContinuousClock.now < deadline { try await Task.sleep(for: .milliseconds(20)) }
        XCTAssertTrue(rt.showFirstEditHint, "直し方は予約される")
        XCTAssertEqual(rt.activeNotice, .summary, "予約されても要約が先")

        rt.summaryBanner = nil
        XCTAssertEqual(rt.activeNotice, .publicData)
        rt.keepPublic()
        XCTAssertEqual(rt.activeNotice, .editHint)
        rt.showFirstEditHint = false
        XCTAssertNil(rt.activeNotice)
    }
}
