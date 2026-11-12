import XCTest
@testable import Metamon

#if METAMON_STG
/// #2079: つまみの読み口。**起動時の環境変数が、メニューで保存した値より強い**(UI テスト・E2E の結果が
/// 端末に残った設定で変わらない)ことと、「全部元に戻す」が `metamon.debug.*` だけを消すことを確かめる。
final class DebugKnobsTests: XCTestCase {
    private var defaults: UserDefaults!
    private let suite = "jp.kz2601.metamon.tests.debugKnobs"

    override func setUp() {
        defaults = UserDefaults(suiteName: suite)
        defaults.removePersistentDomain(forName: suite)
    }

    override func tearDown() {
        defaults.removePersistentDomain(forName: suite)
        defaults = nil
    }

    // MARK: - 読む順番

    func testSavedValueIsUsedWhenNoEnvironment() {
        DebugKnobs.save("example", for: "METAMON_INITIAL_EXAMPLE", defaults: defaults)
        XCTAssertEqual(DebugKnobs.string("METAMON_INITIAL_EXAMPLE", environment: [:], defaults: defaults), "example")
        XCTAssertEqual(DebugKnobs.source("METAMON_INITIAL_EXAMPLE", environment: [:], defaults: defaults), .saved)
    }

    func testEnvironmentWinsOverSavedValue() {
        DebugKnobs.save("saved", for: "METAMON_INITIAL_EXAMPLE", defaults: defaults)
        let env = ["METAMON_INITIAL_EXAMPLE": "fromEnv"]
        XCTAssertEqual(DebugKnobs.string("METAMON_INITIAL_EXAMPLE", environment: env, defaults: defaults), "fromEnv")
        XCTAssertEqual(DebugKnobs.source("METAMON_INITIAL_EXAMPLE", environment: env, defaults: defaults), .environment)
    }

    /// 入 / 切も環境変数が勝つ(保存値 ON・環境変数 OFF なら OFF)
    func testEnvironmentWinsForBool() {
        DebugKnobs.save(true, for: "METAMON_MOCK", defaults: defaults)
        XCTAssertTrue(DebugKnobs.bool("METAMON_MOCK", environment: [:], defaults: defaults))
        XCTAssertFalse(DebugKnobs.bool("METAMON_MOCK", environment: ["METAMON_MOCK": "0"], defaults: defaults))
    }

    /// 空文字の環境変数は「渡していない」と同じ(xcodebuild の既定値が空で入ることがある)
    func testEmptyEnvironmentFallsBackToSavedValue() {
        DebugKnobs.save("saved", for: "METAMON_INITIAL_SCREEN", defaults: defaults)
        XCTAssertEqual(DebugKnobs.string("METAMON_INITIAL_SCREEN", environment: ["METAMON_INITIAL_SCREEN": ""], defaults: defaults),
                       "saved")
    }

    func testNoValueIsNil() {
        XCTAssertNil(DebugKnobs.string("METAMON_NOT_SET", environment: [:], defaults: defaults))
        XCTAssertEqual(DebugKnobs.source("METAMON_NOT_SET", environment: [:], defaults: defaults), .none)
    }

    // MARK: - 値の読み方

    func testTruthy() {
        for raw in ["1", "true", "TRUE", "yes", "On", " 1 "] { XCTAssertEqual(DebugKnobs.truthy(raw), true, raw) }
        for raw in ["0", "false", "no", "OFF"] { XCTAssertEqual(DebugKnobs.truthy(raw), false, raw) }
        XCTAssertNil(DebugKnobs.truthy("maybe"))
    }

    /// 読めない値は既定に戻す(つまみの既定が勝手に反転しない)
    func testBoolFallsBackToDefaultWhenUnreadable() {
        DebugKnobs.save("maybe", for: "METAMON_MOCK", defaults: defaults)
        XCTAssertTrue(DebugKnobs.bool("METAMON_MOCK", default: true, environment: [:], defaults: defaults))
        XCTAssertFalse(DebugKnobs.bool("METAMON_MOCK", default: false, environment: [:], defaults: defaults))
    }

    func testIntAndDouble() {
        XCTAssertEqual(DebugKnobs.int("METAMON_SEED_DUMMY", environment: ["METAMON_SEED_DUMMY": "3"], defaults: defaults), 3)
        XCTAssertNil(DebugKnobs.int("METAMON_SEED_DUMMY", environment: ["METAMON_SEED_DUMMY": "x"], defaults: defaults))
        XCTAssertEqual(DebugKnobs.double("METAMON_MOCK_AI_DELAY", environment: ["METAMON_MOCK_AI_DELAY": "0.5"], defaults: defaults),
                       0.5)
    }

    // MARK: - 保存する / 消す

    func testKeyIsPrefixed() {
        DebugKnobs.save("v", for: "METAMON_MOCK", defaults: defaults)
        XCTAssertEqual(DebugKnobs.defaultsKey("METAMON_MOCK"), "metamon.debug.METAMON_MOCK")
        XCTAssertEqual(defaults.string(forKey: "metamon.debug.METAMON_MOCK"), "v")
    }

    func testSavingEmptyRemovesTheValue() {
        DebugKnobs.save("v", for: "METAMON_MOCK", defaults: defaults)
        DebugKnobs.save("", for: "METAMON_MOCK", defaults: defaults)
        XCTAssertNil(DebugKnobs.string("METAMON_MOCK", environment: [:], defaults: defaults))
        DebugKnobs.save("v", for: "METAMON_MOCK", defaults: defaults)
        DebugKnobs.save(nil, for: "METAMON_MOCK", defaults: defaults)
        XCTAssertNil(DebugKnobs.string("METAMON_MOCK", environment: [:], defaults: defaults))
    }

    // MARK: - 全部元に戻す

    func testResetAllRemovesOnlyDebugKeys() {
        DebugKnobs.save("1", for: "METAMON_MOCK", defaults: defaults)
        DebugKnobs.save("2", for: "METAMON_SEED_DUMMY", defaults: defaults)
        defaults.set(true, forKey: "metamon.pro.limitDisabled") // アプリ本体の設定は消さない
        defaults.set("keep", forKey: "metamon.appId")

        let removed = DebugKnobs.resetAll(defaults: defaults)

        XCTAssertEqual(removed, ["metamon.debug.METAMON_MOCK", "metamon.debug.METAMON_SEED_DUMMY"])
        XCTAssertNil(DebugKnobs.string("METAMON_MOCK", environment: [:], defaults: defaults))
        XCTAssertNil(DebugKnobs.string("METAMON_SEED_DUMMY", environment: [:], defaults: defaults))
        XCTAssertTrue(defaults.bool(forKey: "metamon.pro.limitDisabled"))
        XCTAssertEqual(defaults.string(forKey: "metamon.appId"), "keep")
    }

    /// 元に戻しても環境変数は消せない(渡した起動では戻らないことを UI にも出す)
    func testResetAllCannotUndoEnvironment() {
        DebugKnobs.save("saved", for: "METAMON_INITIAL_EXAMPLE", defaults: defaults)
        DebugKnobs.resetAll(defaults: defaults)
        let env = ["METAMON_INITIAL_EXAMPLE": "fromEnv"]
        XCTAssertEqual(DebugKnobs.string("METAMON_INITIAL_EXAMPLE", environment: env, defaults: defaults), "fromEnv")
        XCTAssertEqual(DebugKnobs.source("METAMON_INITIAL_EXAMPLE", environment: env, defaults: defaults), .environment)
    }

    func testResetAllOnEmptyDefaultsIsNoop() {
        XCTAssertEqual(DebugKnobs.resetAll(defaults: defaults), [])
    }

    // MARK: - つまみの宣言(子の Issue #2080 以降の足し場)

    /// この Issue では枠だけ。足す時に名前がかぶらないことと、名前が `METAMON_` で始まることを守る
    func testCatalogNamesAreUniqueAndPrefixed() {
        let names = DebugKnobCatalog.all.map(\.name)
        XCTAssertEqual(Set(names).count, names.count, "つまみの名前がかぶっています")
        for name in names { XCTAssertTrue(name.hasPrefix("METAMON_"), "環境変数と同じ名前にしてください: \(name)") }
        XCTAssertFalse(DebugKnobCatalog.needsRelaunch("METAMON_NOT_IN_CATALOG"))
    }

    func testEffectLabels() {
        XCTAssertEqual(DebugKnob.Effect.immediate.label, "すぐ効く")
        XCTAssertEqual(DebugKnob.Effect.needsRelaunch.label, "起動し直しが要る")
    }
}
#endif
