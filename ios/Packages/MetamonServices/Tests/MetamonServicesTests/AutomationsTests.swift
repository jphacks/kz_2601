#if !APP_STORE_REVIEW
import XCTest
@testable import MetamonServices

/// #347: GET /v1/apps/{appId}/automations の読み取り。
final class AutomationsTests: XCTestCase {
    func testDecodeFullResponse() throws {
        let json = #"""
        {"appId":"a","notifyPaused":{"until":"2026-11-09T09:00:00.123456Z"},
         "quota":{"used":7,"limit":100,"resetAt":"2026-11-06T15:00:00Z"},
         "rules":[
          {"id":"morning","description":"毎日 07:00、「記録」に記録を足す","trigger":"schedule","result":"limit",
           "lastRunAt":"2026-11-06T07:00:01.5Z","lastOkAt":"2026-11-05T07:00:00Z"},
          {"id":"on_log","description":"「メモ帳」が作られた時、アプリの作成者に通知","trigger":"record","result":"stopped","stopped":{}},
          {"id":"new","description":"x","trigger":"record","result":"none"}
         ]}
        """#
        let a = try JSONDecoder().decode(AppAutomations.self, from: Data(json.utf8))
        XCTAssertEqual(a.appId, "a")
        XCTAssertNil(a.stopped)
        XCTAssertNotNil(a.notifyPaused?.until)
        XCTAssertEqual(a.quota, AutomationQuota(used: 7, limit: 100, resetAt: AppSummary.parseDate("2026-11-06T15:00:00Z")))
        XCTAssertEqual(a.rules.map(\.result), [.limit, .stopped, .none])
        XCTAssertEqual(a.rules[0].trigger, .schedule)
        XCTAssertNotNil(a.rules[0].lastRunAt)
        XCTAssertNotNil(a.rules[0].lastOkAt)
        XCTAssertNotNil(a.rules[1].stopped)
        XCTAssertNil(a.rules[1].stopped?.until)
        XCTAssertNil(a.rules[2].lastRunAt)
    }

    /// 知らない結果の語は other(落とさない)。止まった結果だけが「問題」。
    func testUnknownResultAndProblem() throws {
        let json = #"{"appId":"a","rules":[{"id":"r","description":"x","trigger":"later","result":"brand_new"}]}"#
        let a = try JSONDecoder().decode(AppAutomations.self, from: Data(json.utf8))
        XCTAssertEqual(a.rules[0].result, .other)
        XCTAssertEqual(a.rules[0].trigger, .record)
        XCTAssertNil(a.quota)
        XCTAssertFalse(AutomationRule.Result.ok.isProblem)
        XCTAssertFalse(AutomationRule.Result.none.isProblem)
        XCTAssertTrue(AutomationRule.Result.notifyPaused.isProblem)
        XCTAssertTrue(AutomationRule.Result.failed.isProblem)
        // サーバーの語(openapi AutomationResult)を全部知っている
        for word in ["none", "ok", "notify_paused", "limit", "service_limit", "service_paused", "plan", "stopped",
                     "changed", "unavailable", "late", "inactive", "failed", "other"] {
            XCTAssertNotNil(AutomationRule.Result(rawValue: word), word)
        }
    }
}
#endif
