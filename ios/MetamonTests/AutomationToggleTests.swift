#if !APP_STORE_REVIEW
import XCTest
import MetamonServices
@testable import Metamon

/// #347: 自動の動きのオン / オフ(楽観的に変え、失敗したら戻す・412 は取り直す)。
@MainActor
final class AutomationToggleTests: XCTestCase {
    private final class FakeClient: AutomationsClient, @unchecked Sendable {
        var response: AppAutomations
        var setError: Error?
        var setCalls: [(String, Bool)] = []
        var loadCount = 0

        init(_ response: AppAutomations) { self.response = response }

        func automations(appId: String) async throws -> AppAutomations {
            loadCount += 1
            return response
        }

        func setRule(_ ruleId: String, enabled: Bool, appId: String) async throws {
            setCalls.append((ruleId, enabled))
            if let setError { throw setError }
        }
    }

    private func rules(_ m: AutomationsModel) -> [AutomationRule] {
        if case .loaded(let a) = m.phase { return a.rules }
        return []
    }

    private func make(enabled: Bool? = true) async -> (AutomationsModel, FakeClient) {
        let client = FakeClient(AppAutomations(appId: "a", rules: [AutomationRule(id: "r", description: "x", enabled: enabled)]))
        let m = AutomationsModel(appId: "a", client: client)
        await m.load()
        return (m, client)
    }

    func testTurnOffSucceeds() async {
        let (m, client) = await make()
        await m.setEnabled(false, ruleId: "r")
        XCTAssertEqual(rules(m).first?.enabled, false)
        XCTAssertEqual(client.setCalls.map(\.0), ["r"])
        XCTAssertEqual(client.setCalls.map(\.1), [false])
        XCTAssertNil(m.toggleError)
        XCTAssertTrue(m.pending.isEmpty)
    }

    /// 送れなかったら元に戻して案内(取り直さない)。
    func testRollbackOnFailure() async {
        let (m, client) = await make()
        client.setError = APIError.http(status: 500, body: "")
        await m.setEnabled(false, ruleId: "r")
        XCTAssertEqual(rules(m).first?.enabled, true)
        XCTAssertNotNil(m.toggleError)
        XCTAssertEqual(client.loadCount, 1)
        XCTAssertTrue(m.pending.isEmpty)
    }

    /// 412(他で変わっていた): 戻してから一覧を取り直し、サーバーの状態を見せる。
    func testConflictRefetches() async {
        let (m, client) = await make()
        client.setError = APIError.server(status: 412, code: "precondition_failed", message: "")
        client.response = AppAutomations(appId: "a", rules: [AutomationRule(id: "r", description: "x", enabled: false)])
        await m.setEnabled(false, ruleId: "r")
        XCTAssertEqual(client.loadCount, 2)
        XCTAssertEqual(rules(m).first?.enabled, false) // 取り直した値
        XCTAssertNotNil(m.toggleError)
    }

    /// enabled を返さない古いサーバー・同じ状態では送らない。
    func testNoOpWithoutEnabledOrSameState() async {
        let (old, oldClient) = await make(enabled: nil)
        await old.setEnabled(false, ruleId: "r")
        XCTAssertTrue(oldClient.setCalls.isEmpty)

        let (m, client) = await make(enabled: true)
        await m.setEnabled(true, ruleId: "r")
        XCTAssertTrue(client.setCalls.isEmpty)
    }

    func testFixRequestMentionsRule() {
        let text = AutomationsModel.fixRequest(for: AutomationRule(id: "r", description: "毎日 07:00、通知"))
        XCTAssertTrue(text.contains("毎日 07:00、通知"))
    }
}
#endif
