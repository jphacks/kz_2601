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
                     "changed", "unavailable", "late", "inactive", "failed", "disabled", "other"] {
            XCTAssertNotNil(AutomationRule.Result(rawValue: word), word)
        }
        // 作成者がオフにした結果は自分の操作(お問い合わせは出さない)
        XCTAssertFalse(AutomationRule.Result.disabled.isProblem)
    }

    // MARK: - オン / オフ(#347・backend #693)

    /// enabled / consecutiveFailures を読む。古いサーバー(どちらも無い)では enabled = nil(切り替えを出さない)・失敗 0。
    func testDecodeEnabledAndFailures() throws {
        let json = #"""
        {"appId":"a","rules":[
          {"id":"off","description":"x","trigger":"record","result":"disabled","enabled":false},
          {"id":"failing","description":"y","trigger":"schedule","result":"failed","enabled":true,"consecutiveFailures":3},
          {"id":"old","description":"z","trigger":"record","result":"ok"},
          {"id":"weird","description":"w","trigger":"record","result":"ok","enabled":"yes","consecutiveFailures":-2}
        ]}
        """#
        let r = try JSONDecoder().decode(AppAutomations.self, from: Data(json.utf8)).rules
        XCTAssertEqual(r[0].enabled, false)
        XCTAssertEqual(r[0].result, .disabled)
        XCTAssertEqual(r[0].consecutiveFailures, 0)
        XCTAssertFalse(r[0].suggestsFix)
        XCTAssertEqual(r[1].enabled, true)
        XCTAssertEqual(r[1].consecutiveFailures, 3)
        XCTAssertTrue(r[1].suggestsFix)
        XCTAssertNil(r[2].enabled)
        XCTAssertEqual(r[2].consecutiveFailures, 0)
        XCTAssertNil(r[3].enabled) // 型違いでも一覧ごと落とさない
        XCTAssertEqual(r[3].consecutiveFailures, 0)
    }

    /// failureReason を読む。知らない語・型違い・failed 以外の結果・古いサーバー(無し)は nil(いつもの文言だけ)。
    func testDecodeFailureReason() throws {
        let json = #"""
        {"appId":"a","rules":[
          {"id":"notify","description":"x","trigger":"record","result":"failed","failureReason":"notify_failed"},
          {"id":"new","description":"x","trigger":"record","result":"failed","failureReason":"brand_new_reason"},
          {"id":"old","description":"x","trigger":"record","result":"failed"},
          {"id":"weird","description":"x","trigger":"record","result":"failed","failureReason":3},
          {"id":"ok","description":"x","trigger":"record","result":"ok","failureReason":"timeout"}
        ]}
        """#
        let r = try JSONDecoder().decode(AppAutomations.self, from: Data(json.utf8)).rules
        XCTAssertEqual(r.count, 5)
        XCTAssertEqual(r[0].failureReason, .notifyFailed)
        XCTAssertNil(r[1].failureReason)
        XCTAssertEqual(r[1].result, .failed)
        XCTAssertNil(r[2].failureReason)
        XCTAssertNil(r[3].failureReason)
        XCTAssertNil(r[4].failureReason)
        // サーバーの語(openapi AutomationFailureReason)を全部知っている
        for word in ["internal", "mismatch", "notify_failed", "record_unavailable", "source_unavailable", "timeout", "write_rejected"] {
            XCTAssertNotNil(AutomationRule.FailureReason(rawValue: word), word)
        }
        XCTAssertEqual(AutomationRule.FailureReason.allCases.count, 7)
        XCTAssertNil(AutomationRule(id: "r", description: "x", result: .ok, failureReason: .timeout).failureReason)
    }

    func testSuggestsFixThreshold() {
        XCTAssertFalse(AutomationRule(id: "r", description: "x", consecutiveFailures: 2).suggestsFix)
        XCTAssertTrue(AutomationRule(id: "r", description: "x", consecutiveFailures: 3).suggestsFix)
    }

    /// GET /dsl の封筒から version と serverRules。
    func testParseEnvelope() throws {
        let json = #"{"appId":"a","version":7,"dsl":{"serverRules":[{"id":"a"},{"id":"b","enabled":false}]}}"#
        let p = try AutomationToggle.parse(envelope: Data(json.utf8))
        XCTAssertEqual(p.version, 7)
        XCTAssertEqual(p.serverRules.count, 2)
        XCTAssertEqual(try AutomationToggle.parse(envelope: Data(#"{"version":2,"dsl":{}}"#.utf8)).serverRules.count, 0)
        XCTAssertThrowsError(try AutomationToggle.parse(envelope: Data(#"{"dsl":{}}"#.utf8)))
    }

    /// オフ = add enabled:false、オン = remove enabled。位置は id で引く。もうその状態なら PATCH しない。
    func testPatchOps() throws {
        let rules: [[String: Any]] = [["id": "a"], ["id": "b", "enabled": false], ["id": "c", "enabled": true]]
        XCTAssertEqual(try AutomationToggle.ops(ruleId: "a", enabled: false, serverRules: rules),
                       [.init(op: "add", path: "/serverRules/0/enabled", value: false)])
        XCTAssertEqual(try AutomationToggle.ops(ruleId: "b", enabled: true, serverRules: rules),
                       [.init(op: "remove", path: "/serverRules/1/enabled")])
        XCTAssertEqual(try AutomationToggle.ops(ruleId: "c", enabled: false, serverRules: rules),
                       [.init(op: "add", path: "/serverRules/2/enabled", value: false)])
        XCTAssertEqual(try AutomationToggle.ops(ruleId: "c", enabled: true, serverRules: rules),
                       [.init(op: "remove", path: "/serverRules/2/enabled")])
        XCTAssertEqual(try AutomationToggle.ops(ruleId: "a", enabled: true, serverRules: rules), [])
        XCTAssertEqual(try AutomationToggle.ops(ruleId: "b", enabled: false, serverRules: rules), [])
        XCTAssertThrowsError(try AutomationToggle.ops(ruleId: "gone", enabled: false, serverRules: rules)) {
            XCTAssertEqual($0 as? AutomationToggle.Failure, .ruleNotFound)
        }
    }

    /// 本文は {"patch":[...]}。remove には value を付けない。
    func testPatchBodyEncoding() throws {
        let body = AutomationToggle.Body(patch: [.init(op: "add", path: "/serverRules/0/enabled", value: false),
                                                 .init(op: "remove", path: "/serverRules/1/enabled")])
        let obj = try XCTUnwrap(JSONSerialization.jsonObject(with: JSONEncoder().encode(body)) as? [String: Any])
        let ops = try XCTUnwrap(obj["patch"] as? [[String: Any]])
        XCTAssertEqual(ops[0]["op"] as? String, "add")
        XCTAssertEqual(ops[0]["value"] as? Bool, false)
        XCTAssertEqual(ops[1]["op"] as? String, "remove")
        XCTAssertNil(ops[1]["value"])
    }

    func testConflictClassification() {
        XCTAssertTrue(AutomationToggle.isConflict(APIError.server(status: 412, code: "precondition_failed", message: "")))
        XCTAssertTrue(AutomationToggle.isConflict(APIError.http(status: 409, body: "")))
        XCTAssertTrue(AutomationToggle.isConflict(AutomationToggle.Failure.ruleNotFound))
        XCTAssertFalse(AutomationToggle.isConflict(APIError.http(status: 500, body: "")))
        XCTAssertFalse(AutomationToggle.isConflict(URLError(.notConnectedToInternet)))
    }
}
#endif
