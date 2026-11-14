import XCTest
import MetamonServices
@testable import Metamon

/// 自動の動きのオン / オフ(#347 の Mac 版・#979): 先に見た目を変え、失敗したら戻す
@MainActor
final class MacAutomationsTests: XCTestCase {
    private actor FakeClient: AutomationsClient {
        let rules: [AutomationRule]
        let failWith: Error?
        private(set) var sent: [String] = []
        init(rules: [AutomationRule], failWith: Error? = nil) {
            self.rules = rules
            self.failWith = failWith
        }
        func automations(appId: String) async throws -> AppAutomations { AppAutomations(appId: appId, rules: rules) }
        func setRule(_ ruleId: String, enabled: Bool, appId: String) async throws {
            sent.append("\(ruleId)=\(enabled)")
            if let failWith { throw failWith }
        }
    }

    private func enabled(_ model: AutomationsModel, _ id: String) -> Bool? {
        guard case .loaded(let a) = model.phase else { return nil }
        return a.rules.first { $0.id == id }?.enabled
    }

    func testToggleKeepsNewStateOnSuccess() async {
        let client = FakeClient(rules: [AutomationRule(id: "r1", description: "通知", enabled: true)])
        let model = AutomationsModel(appId: "a", client: client)
        await model.load()
        await model.setEnabled(false, ruleId: "r1")
        XCTAssertEqual(enabled(model, "r1"), false)
        XCTAssertNil(model.toggleError)
        let sent = await client.sent
        XCTAssertEqual(sent, ["r1=false"])
    }

    func testToggleRollsBackOnFailure() async {
        let client = FakeClient(rules: [AutomationRule(id: "r1", description: "通知", enabled: true)],
                                failWith: URLError(.notConnectedToInternet))
        let model = AutomationsModel(appId: "a", client: client)
        await model.load()
        await model.setEnabled(false, ruleId: "r1")
        XCTAssertEqual(enabled(model, "r1"), true, "失敗したら戻す")
        XCTAssertNotNil(model.toggleError)
        XCTAssertTrue(model.pending.isEmpty)
    }

    /// 古いサーバー(enabled を返さない)には切り替えを送らない
    func testNoToggleWithoutEnabled() async {
        let client = FakeClient(rules: [AutomationRule(id: "r1", description: "通知")])
        let model = AutomationsModel(appId: "a", client: client)
        await model.load()
        await model.setEnabled(false, ruleId: "r1")
        let sent = await client.sent
        XCTAssertTrue(sent.isEmpty)
    }

    func testFixRequestNamesRule() {
        let rule = AutomationRule(id: "r1", description: "毎朝まとめを通知", result: .failed, consecutiveFailures: 3)
        XCTAssertTrue(rule.suggestsFix)
        XCTAssertTrue(AutomationsModel.fixRequest(for: rule).contains("毎朝まとめを通知"))
    }
}
