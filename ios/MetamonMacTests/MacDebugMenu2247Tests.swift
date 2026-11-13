import XCTest
@testable import Metamon

/// #2247: 開発用メニューの判定とつまみを Mac でも同じ決まりで使う(本体は ios/Shared/Debug・iOS の DebugMenuPolicyTests / DebugKnobsTests と同じケース)。
final class MacDebugMenuPolicy2247Tests: XCTestCase {
    private let stgAI = URL(string: "https://ai-stg-378953239426.asia-northeast1.run.app/")
    private let stgAppConfig = URL(string: "https://appconfig-stg-378953239426.asia-northeast1.run.app/")
    private let prodAI = URL(string: "https://ai-prod-378953239426.asia-northeast1.run.app/")
    private let prodAppConfig = URL(string: "https://appconfig-prod-378953239426.asia-northeast1.run.app/")

    func testShownForStgAndMock() {
        XCTAssertTrue(DebugMenuPolicy.isAvailable(compiledIn: true, aiURL: stgAI, appConfigURL: stgAppConfig))
        XCTAssertTrue(DebugMenuPolicy.isAvailable(compiledIn: true, aiURL: nil, appConfigURL: nil))
    }

    func testHiddenWhenEitherURLIsProd() {
        XCTAssertFalse(DebugMenuPolicy.isAvailable(compiledIn: true, aiURL: prodAI, appConfigURL: stgAppConfig))
        XCTAssertFalse(DebugMenuPolicy.isAvailable(compiledIn: true, aiURL: stgAI, appConfigURL: prodAppConfig))
        XCTAssertFalse(DebugMenuPolicy.isAvailable(compiledIn: true, aiURL: nil, appConfigURL: prodAppConfig))
    }

    func testHiddenWithoutCompilationCondition() {
        XCTAssertFalse(DebugMenuPolicy.isAvailable(compiledIn: false, aiURL: stgAI, appConfigURL: stgAppConfig))
    }

    /// Mac も Debug / Staging だけ METAMON_STG(project.yml)。テストは Debug で回すので true
    func testCompiledInMatchesCondition() {
        #if METAMON_STG
        XCTAssertTrue(DebugMenuPolicy.isCompiledIn)
        #else
        XCTAssertFalse(DebugMenuPolicy.isCompiledIn)
        XCTAssertFalse(DebugMenuPolicy.isAvailable)
        #endif
    }

    /// Mac の接続先は MacServices と同じ読み方
    func testMacReadsSameURLsAsMacServices() {
        XCTAssertEqual(DebugMenuPolicy.currentAIURL, MacServices.url(env: "METAMON_AI_BASE_URL", plist: "MetamonAIBaseURL"))
        XCTAssertEqual(DebugMenuPolicy.currentAppConfigURL,
                       MacServices.url(env: "METAMON_APPCONFIG_BASE_URL", plist: "MetamonAppConfigBaseURL"))
    }
}

#if METAMON_STG
final class MacDebugKnobs2247Tests: XCTestCase {
    private var suiteName = ""
    private var defaults: UserDefaults!
    private var env: [String: String] = [:]

    override func setUp() {
        suiteName = "MacDebugKnobs2247Tests.\(UUID().uuidString)"
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

    func testEnvironmentWinsOverSavedValue() {
        store.set("METAMON_FOO", "saved")
        XCTAssertEqual(store.string("METAMON_FOO"), "saved")
        XCTAssertEqual(defaults.string(forKey: "metamon.debug.METAMON_FOO"), "saved")
        env["METAMON_FOO"] = "env"
        XCTAssertEqual(store.string("METAMON_FOO"), "env")
        XCTAssertTrue(store.isFromEnvironment("METAMON_FOO"))
    }

    func testBoolParsing() {
        store.set("METAMON_A", true)
        XCTAssertEqual(store.bool("METAMON_A"), true)
        env["METAMON_B"] = "no"
        XCTAssertEqual(store.bool("METAMON_B"), false)
        env["METAMON_B"] = "maybe"
        XCTAssertNil(store.bool("METAMON_B"))
    }

    func testResetAllRemovesOnlyDebugKeys() {
        store.set("METAMON_FOO", "1")
        defaults.set("keep", forKey: "metamon.other")
        store.resetAll()
        XCTAssertNil(defaults.object(forKey: "metamon.debug.METAMON_FOO"))
        XCTAssertEqual(defaults.string(forKey: "metamon.other"), "keep")
    }

    func testSharedStoreUsesDebugSuite() {
        XCTAssertEqual(DebugKnobs.suiteName, "jp.kz2601.metamon.debug")
        XCTAssertNotEqual(DebugKnobs.store.defaults, UserDefaults.standard)
    }
}
#endif
