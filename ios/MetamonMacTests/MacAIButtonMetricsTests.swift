import XCTest
import MetamonServices
@testable import Metamon

/// 端末内 AI の集計を Mac から送る(#1327)
@MainActor
final class MacAIButtonMetricsTests: XCTestCase {
    private actor Spy: AIButtonMetricsSender {
        var reports: [AIButtonMetricsReport] = []
        func send(_ report: AIButtonMetricsReport) async throws { reports.append(report) }
    }

    private func recorder() -> AIButtonMetricsRecorder {
        let name = "mac-aibutton-\(UUID().uuidString)"
        let defaults = UserDefaults(suiteName: name)!
        defaults.removePersistentDomain(forName: name)
        return AIButtonMetricsRecorder(defaults: defaults)
    }

    func testFlushSendsPreviousDays() async {
        let rec = recorder()
        rec.record(.shown(.available), now: Date().addingTimeInterval(-2 * 86_400))
        let spy = Spy()
        await MacAIButtonMetrics.flush(sender: spy, recorder: rec)
        let sent = await spy.reports
        XCTAssertEqual(sent.count, 1)
        XCTAssertNil(rec.pendingReport(), "送れた分は消す")
    }

    /// サーバーにつながっていない(モック)時は送らずに貯めたまま
    func testNoSenderKeepsBuckets() async {
        let rec = recorder()
        rec.record(.shown(.available), now: Date().addingTimeInterval(-2 * 86_400))
        await MacAIButtonMetrics.flush(sender: nil, recorder: rec)
        XCTAssertNotNil(rec.pendingReport())
    }

    func testMockServicesHaveNoSender() {
        let services = MacServices.make(environment: ["METAMON_MOCK": "1"])
        XCTAssertNil(services.aiButtonMetrics)
    }
}
