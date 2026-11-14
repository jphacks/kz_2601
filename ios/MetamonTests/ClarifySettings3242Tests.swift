import XCTest
@testable import Metamon

/// #3242: 生成の前に相談するかの設定(`ClarifySettings`)。UserDefaults の鍵は最後に元へ戻す
final class ClarifySettings3242Tests: XCTestCase {
    private let enabledKey = "metamon.clarify.enabled"
    private let autoBuildKey = "metamon.clarify.autoBuild"
    private var savedEnabled: Any?
    private var savedAutoBuild: Any?

    override func setUpWithError() throws {
        // 環境変数の上書きがある実行では UserDefaults の値を検証できない
        let env = ProcessInfo.processInfo.environment
        try XCTSkipIf(env["METAMON_CLARIFY"] != nil || env["METAMON_CLARIFY_AUTOBUILD"] != nil, "環境変数が優先される")
        savedEnabled = UserDefaults.standard.object(forKey: enabledKey)
        savedAutoBuild = UserDefaults.standard.object(forKey: autoBuildKey)
        UserDefaults.standard.removeObject(forKey: enabledKey)
        UserDefaults.standard.removeObject(forKey: autoBuildKey)
    }

    override func tearDown() {
        restore(enabledKey, savedEnabled)
        restore(autoBuildKey, savedAutoBuild)
    }

    private func restore(_ key: String, _ value: Any?) {
        if let value { UserDefaults.standard.set(value, forKey: key) } else { UserDefaults.standard.removeObject(forKey: key) }
    }

    func testEnabledDefaultsToOn() {
        XCTAssertTrue(ClarifySettings.enabled)
    }

    func testEnabledReadsSavedValue() {
        ClarifySettings.enabled = false
        XCTAssertFalse(ClarifySettings.enabled)
        XCTAssertEqual(UserDefaults.standard.object(forKey: enabledKey) as? Bool, false)
        ClarifySettings.enabled = true
        XCTAssertTrue(ClarifySettings.enabled)
    }

    func testAutoBuildDefaultsToOn() {
        XCTAssertTrue(ClarifySettings.autoBuild)
    }

    func testAutoBuildReadsSavedValueIndependently() {
        ClarifySettings.autoBuild = false
        XCTAssertFalse(ClarifySettings.autoBuild)
        XCTAssertTrue(ClarifySettings.enabled, "autoBuild を変えても enabled は動かない")
        UserDefaults.standard.removeObject(forKey: autoBuildKey)
        XCTAssertTrue(ClarifySettings.autoBuild)
    }
}
