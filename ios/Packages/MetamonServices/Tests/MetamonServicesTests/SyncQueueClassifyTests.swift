import XCTest
import MetamonDSL
@testable import MetamonServices

/// 送信待ちの失敗の分け方。`SyncQueue.classify`(再送する / 競合 / 作者以外 / 諦める)と
/// `SyncQueue.refusal`(再試行では直らない断り。classify より先に見る)を APIError の表で確かめる。
final class SyncQueueClassifyTests: XCTestCase {
    private func http(_ status: Int) -> APIError { .http(status: status, body: "") }
    private func server(_ status: Int, _ code: String, _ message: String = "", _ details: [String: JSONValue] = [:]) -> APIError {
        .server(status: status, code: code, message: message, details: details)
    }

    // MARK: classify

    func testTransientStatusesAreRetried() {
        for status in [401, 408, 425, 429, 500, 502, 503, 504, 599] {
            XCTAssertEqual(SyncQueue.classify(http(status)), .transient, "HTTP \(status) は再送する")
        }
    }

    func testNonAPIErrorsAreRetried() {
        XCTAssertEqual(SyncQueue.classify(URLError(.notConnectedToInternet)), .transient)
        XCTAssertEqual(SyncQueue.classify(URLError(.timedOut)), .transient)
        XCTAssertEqual(SyncQueue.classify(CocoaError(.fileReadUnknown)), .transient)
    }

    func testInvalidURLHasNoStatusSoItIsRetried() {
        XCTAssertEqual(SyncQueue.classify(APIError.invalidURL), .transient, "status が無いものは通信断と同じ扱い")
    }

    func testConflictStatuses() {
        for status in [404, 409, 412] {
            XCTAssertEqual(SyncQueue.classify(http(status)), .conflict, "HTTP \(status) は競合(サーバー優先)")
        }
        XCTAssertEqual(SyncQueue.classify(server(409, "conflict")), .conflict)
    }

    func testForbiddenIs403() {
        XCTAssertEqual(SyncQueue.classify(http(403)), .forbidden)
        XCTAssertEqual(SyncQueue.classify(server(403, "forbidden", "only the author can modify this record")), .forbidden)
    }

    func testOtherClientErrorsAreRejectedWithMessage() {
        XCTAssertEqual(SyncQueue.classify(server(422, "invalid_record", "bad amount")), .rejected("bad amount"))
        XCTAssertEqual(SyncQueue.classify(server(400, "bad_request", "")), .rejected("bad_request"), "message が空なら code")
        XCTAssertEqual(SyncQueue.classify(server(402, "pro_required", "needs pro")), .rejected("needs pro"), "402 も諦める(再送しても直らない)")
        XCTAssertEqual(SyncQueue.classify(http(410)), .rejected("HTTP 410: "))
        XCTAssertEqual(SyncQueue.classify(http(499)), .rejected("HTTP 499: "))
    }

    func testRejectedMessageForHTTPIncludesBodyPrefix() {
        XCTAssertEqual(SyncQueue.classify(APIError.http(status: 400, body: "nope")), .rejected("HTTP 400: nope"))
    }

    func testStatusBoundaries() {
        XCTAssertEqual(SyncQueue.classify(http(399)), .transient)
        XCTAssertEqual(SyncQueue.classify(http(400)), .rejected("HTTP 400: "))
        XCTAssertEqual(SyncQueue.classify(http(500)), .transient)
    }

    // MARK: refusal

    private func refusal(_ error: Error, recordId: String? = "r1") -> SyncEvent? {
        SyncQueue.refusal(error, appId: "a", entity: "memo", recordId: recordId)
    }

    func testRefusalIsNilForNonAPIErrors() {
        XCTAssertNil(refusal(URLError(.notConnectedToInternet)))
        XCTAssertNil(refusal(APIError.invalidURL))
    }

    func testRefusalIsNilForPlainStatuses() {
        for status in [401, 403, 404, 409, 422, 500] {
            XCTAssertNil(refusal(http(status)), "HTTP \(status) だけでは文言を分けない")
        }
        XCTAssertNil(refusal(server(403, "forbidden", "only the author can modify this record")))
    }

    func testPlanLimitRefusal() {
        let e = server(403, "plan_limit_reached", "limit", ["resource": .string("recordsPerApp"), "plan": .string("free"),
                                                            "limit": .number(100), "upgrade": .bool(true)])
        let expected = PlanLimit(resource: .recordsPerApp, plan: "free", limit: 100, upgrade: true)
        XCTAssertEqual(refusal(e), .planLimit(appId: "a", entity: "memo", limit: expected, recordId: "r1"))
    }

    func testPlanLimitCreatorReason() {
        let e = server(403, "plan_limit_reached", "limit", ["resource": .string("photosPerApp"), "reason": .string("creator_plan_limit")])
        guard case .planLimit(_, _, let limit, _)? = refusal(e) else { return XCTFail("planLimit") }
        XCTAssertEqual(limit.resource, .photosPerApp)
        XCTAssertTrue(limit.byCreator, "作成者のプランの上限")
    }

    func testPlanLimitWrongStatusIsNotARefusal() {
        XCTAssertNil(refusal(server(402, "plan_limit_reached")), "403 以外は planLimit にしない")
    }

    func testAppStoppedRefusal() {
        XCTAssertEqual(refusal(server(403, "app_stopped")), .appStopped(appId: "a", entity: "memo", recordId: "r1"))
        XCTAssertNil(refusal(server(500, "app_stopped")), "status が違えば断りではない")
    }

    func testTextNotAllowedRefusalCarriesField() {
        let e = server(422, "text_not_allowed", "ng", ["field": .string("title")])
        XCTAssertEqual(refusal(e), .textNotAllowed(appId: "a", entity: "memo", field: "title", recordId: "r1"))
        XCTAssertEqual(refusal(server(422, "text_not_allowed")), .textNotAllowed(appId: "a", entity: "memo", field: nil, recordId: "r1"))
        XCTAssertNil(refusal(server(400, "text_not_allowed")))
    }

    func testRefusalWithoutRecordId() {
        XCTAssertEqual(refusal(server(403, "app_stopped"), recordId: nil), .appStopped(appId: "a", entity: "memo", recordId: nil))
    }

    func testRefusalPrecedesClassifyForTheSameError() {
        // 403 の app_stopped は classify だけだと .forbidden(作った人以外)。断りを先に見るので別の文言になる
        let e = server(403, "app_stopped")
        XCTAssertEqual(SyncQueue.classify(e), .forbidden)
        XCTAssertNotNil(refusal(e))
        // 422 の text_not_allowed は classify だと .rejected
        let t = server(422, "text_not_allowed", "ng")
        XCTAssertEqual(SyncQueue.classify(t), .rejected("ng"))
        XCTAssertNotNil(refusal(t))
    }
}
