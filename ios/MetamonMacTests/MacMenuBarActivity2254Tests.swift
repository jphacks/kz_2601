import XCTest
import MetamonDSL
@testable import MetamonRenderer
import MetamonServices
@testable import Metamon

/// タイマー・記録中の表示をメニューバーに出す試作(#2254): フラグ off で今と同じ・on で経過時間を出して止められる
@MainActor
final class MacMenuBarActivity2254Tests: XCTestCase {
    private let t0 = Date(timeIntervalSince1970: 1_800_000_000)

    private func action(_ json: String) throws -> Action {
        try JSONDecoder().decode(Action.self, from: Data(json.utf8))
    }

    // MARK: - フラグ off(既定)

    func testFlagIsOffByDefault() throws {
        let suite = "MacMenuBarActivity2254Tests.\(UUID().uuidString)"
        let defaults = try XCTUnwrap(UserDefaults(suiteName: suite))
        defer { defaults.removePersistentDomain(forName: suite) }
        XCTAssertFalse(MacMenuBarActivity.isFlagOn(defaults: defaults, environment: [:]), "既定は off")
        XCTAssertTrue(MacMenuBarActivity.isFlagOn(defaults: defaults, environment: [MacMenuBarActivity.flagEnv: "1"]))
        defaults.set(true, forKey: MacMenuBarActivity.flagKey)
        XCTAssertTrue(MacMenuBarActivity.isFlagOn(defaults: defaults, environment: [:]))
    }

    func testFlagOffDoesNothing() {
        var changes = 0
        let activity = MacMenuBarActivity(isEnabled: { false })
        activity.onChange = { changes += 1 }
        XCTAssertFalse(activity.start(appId: "a", appName: "勉強", spec: ActivitySpec(title: "勉強中"), now: t0))
        activity.timer(appId: "a", appName: "勉強", update: TimerActivityUpdate(key: "a.t", label: "集中", total: nil,
                                                                              run: TimerRun(base: 0, startedAt: t0)))
        activity.end(appId: "a")
        XCTAssertNil(activity.current)
        XCTAssertEqual(changes, 0, "メニューバーには何も出さない")
    }

    func testRuntimeStartActivityStillToastsWhenFlagOff() async throws {
        try XCTSkipIf(MacMenuBarActivity.isFlagOn(), "この Mac で試作のフラグが on")
        let url = try XCTUnwrap(Bundle(for: MacAppRuntime.self).url(forResource: "kakeibo", withExtension: "json"))
        let rt = MacAppRuntime(appId: "act-\(UUID().uuidString.lowercased())", dsl: try AppDSL.load(contentsOf: url),
                               ai: MockAIClient(delay: .zero), isAIConsentGranted: { false })
        let home = Route(screen: "home", stack: Route.tabStack("home"))
        await rt.run(try action(#"{"type":"startActivity","activity":{"title":"記録中"}}"#), ActionContext(route: home))
        XCTAssertEqual(rt.toast, L10n.tr("ライブアクティビティは iPhone だけで表示されます"), "今までと同じ一言")
        XCTAssertNil(MacMenuBarActivity.shared.current)
        await rt.run(try action(#"{"type":"endActivity"}"#), ActionContext(route: home))
        XCTAssertNil(MacMenuBarActivity.shared.current)
    }

    // MARK: - フラグ on

    func testStartShowsElapsedAndStopClears() {
        let activity = MacMenuBarActivity(isEnabled: { true })
        XCTAssertTrue(activity.start(appId: "a", appName: "勉強", spec: ActivitySpec(title: "勉強中", icon: "book"), now: t0))
        let item = try? XCTUnwrap(activity.current)
        XCTAssertEqual(item?.title, "勉強中")
        XCTAssertEqual(item?.icon, "book")
        XCTAssertEqual(item.map { MacMenuBarActivity.statusTitle($0, now: t0.addingTimeInterval(754)) }, "12:34")
        activity.stop()
        XCTAssertNil(activity.current, "止めるで消える")
    }

    func testCountdownShowsRemaining() {
        let activity = MacMenuBarActivity(isEnabled: { true })
        activity.start(appId: "a", appName: "料理", spec: ActivitySpec(title: "煮込み", minutes: 10), now: t0)
        let item = activity.current!
        XCTAssertEqual(MacMenuBarActivity.statusTitle(item, now: t0.addingTimeInterval(60)), "9:00")
        XCTAssertEqual(MacMenuBarActivity.statusTitle(item, now: t0.addingTimeInterval(9999)), "0:00", "0 未満にしない")
    }

    func testOnlyOneAtATimeAndEndOnlyRemovesOwn() {
        let activity = MacMenuBarActivity(isEnabled: { true })
        activity.start(appId: "a", appName: "A", spec: ActivitySpec(title: "A 中"), now: t0)
        activity.timer(appId: "b", appName: "B", update: TimerActivityUpdate(key: "b.t", label: "集中", total: nil,
                                                                          run: TimerRun(base: 0, startedAt: t0)))
        XCTAssertEqual(activity.current?.key, "b.t", "後から来た方に替わる(同時に 1 つ)")
        activity.end(appId: "a")
        XCTAssertEqual(activity.current?.key, "b.t", "endActivity はそのアプリの startActivity の表示だけ消す")
        activity.timer(appId: "b", appName: "B", update: TimerActivityUpdate(key: "b.t", label: "集中", total: nil, run: TimerRun()))
        XCTAssertNil(activity.current, "タイマーのリセットで消える")
    }

    func testTimerPauseShowsFrozenSeconds() {
        let activity = MacMenuBarActivity(isEnabled: { true })
        activity.timer(appId: "b", appName: "B", update: TimerActivityUpdate(key: "b.t", label: "集中", total: nil, run: TimerRun(base: 3725)))
        XCTAssertEqual(activity.current.map { MacMenuBarActivity.statusTitle($0, now: t0) }, "1:02:05")
    }

    func testRemoveWhenAppDeleted() {
        let activity = MacMenuBarActivity(isEnabled: { true })
        activity.start(appId: "a", appName: "A", spec: ActivitySpec(title: "A 中"), now: t0)
        activity.remove(appId: "other")
        XCTAssertNotNil(activity.current)
        activity.remove(appId: "a")
        XCTAssertNil(activity.current)
    }
}
