#if METAMON_STG
import XCTest
@testable import Metamon

/// #2079: 開発用のつまみ(環境変数が保存値より優先・全部元に戻す)。
final class DebugKnobsTests: XCTestCase {
    private var suiteName = ""
    private var defaults: UserDefaults!
    private var env: [String: String] = [:]

    override func setUp() {
        suiteName = "DebugKnobsTests.\(UUID().uuidString)"
        defaults = UserDefaults(suiteName: suiteName)
        env = [:]
    }

    override func tearDown() {
        defaults.removePersistentDomain(forName: suiteName)
        defaults = nil
    }

    private var store: DebugKnobStore {
        DebugKnobStore(defaults: defaults, environment: { [env] in env })
    }

    func testSavedValueIsReadWithPrefixedKey() {
        store.set("METAMON_FOO", "bar")
        XCTAssertEqual(defaults.string(forKey: "metamon.debug.METAMON_FOO"), "bar")
        XCTAssertEqual(store.string("METAMON_FOO"), "bar")
        XCTAssertFalse(store.isFromEnvironment("METAMON_FOO"))
    }

    func testEnvironmentWinsOverSavedValue() {
        store.set("METAMON_FOO", "saved")
        env["METAMON_FOO"] = "env"
        XCTAssertEqual(store.string("METAMON_FOO"), "env")
        XCTAssertTrue(store.isFromEnvironment("METAMON_FOO"))
    }

    func testMissingIsNil() {
        XCTAssertNil(store.string("METAMON_NONE"))
        XCTAssertNil(store.bool("METAMON_NONE"))
    }

    func testBoolParsing() {
        store.set("METAMON_A", true)
        XCTAssertEqual(store.bool("METAMON_A"), true)
        store.set("METAMON_A", false)
        XCTAssertEqual(store.bool("METAMON_A"), false)
        env["METAMON_B"] = "1"
        XCTAssertEqual(store.bool("METAMON_B"), true)
        env["METAMON_B"] = "TRUE"
        XCTAssertEqual(store.bool("METAMON_B"), true)
        env["METAMON_B"] = "no"
        XCTAssertEqual(store.bool("METAMON_B"), false)
        env["METAMON_B"] = "maybe"
        XCTAssertNil(store.bool("METAMON_B"))
    }

    func testSetNilRemoves() {
        store.set("METAMON_FOO", "bar")
        store.set("METAMON_FOO", nil as String?)
        XCTAssertNil(defaults.object(forKey: "metamon.debug.METAMON_FOO"))
    }

    func testResetAllRemovesOnlyDebugKeys() {
        store.set("METAMON_FOO", "1")
        store.set("METAMON_BAR", "2")
        defaults.set("keep", forKey: "metamon.other")
        env["METAMON_FOO"] = "env"
        store.resetAll()
        XCTAssertNil(defaults.object(forKey: "metamon.debug.METAMON_FOO"))
        XCTAssertNil(defaults.object(forKey: "metamon.debug.METAMON_BAR"))
        XCTAssertEqual(defaults.string(forKey: "metamon.other"), "keep")
        XCTAssertEqual(store.string("METAMON_FOO"), "env", "環境変数は元に戻した後も効く")
        XCTAssertNil(store.string("METAMON_BAR"))
    }

    /// 保存先は stg の版だけのスイート(普段の UserDefaults.standard に混ぜない)
    func testSharedStoreUsesDebugSuite() {
        XCTAssertEqual(DebugKnobs.suiteName, "jp.kz2601.metamon.debug")
        XCTAssertNotEqual(DebugKnobs.store.defaults, UserDefaults.standard)
    }
}
#endif
