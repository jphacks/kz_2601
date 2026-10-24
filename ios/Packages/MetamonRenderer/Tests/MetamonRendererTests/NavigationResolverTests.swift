import XCTest
import MetamonDSL
@testable import MetamonRenderer

/// #113: navigate は行き先に応じて pop / タブ切替 / push を選ぶ。
final class NavigationResolverTests: XCTestCase {
    private func example(_ name: String) throws -> AppDSL {
        var url = URL(fileURLWithPath: #filePath)
        for _ in 0..<6 { url.deleteLastPathComponent() }
        return try AppDSL.load(contentsOf: url.appending(path: "docs/dsl/examples/\(name).json"))
    }

    private let tabA = Route.tabStack("a")
    private let tabOther = Route.tabStack("other")

    /// 今見えている画面(スタックの最上段、無ければ根)。
    private func top(_ s: NavigationState, stack: String, root: String) -> Route {
        s.paths[stack]?.last ?? Route(screen: root, stack: stack)
    }

    private func go(_ s: NavigationState, _ screen: String, from: Route, _ dsl: AppDSL,
                    presentation: Presentation? = nil, recordId: String? = nil) -> NavigationState {
        NavigationResolver.navigate(s, to: screen, presentation: presentation, recordId: recordId, from: from, dsl: dsl)
    }

    func testCycleDoesNotDeepen() throws {
        let dsl = try example("navigation")
        var s = NavigationState(selectedTab: "a")
        for _ in 0..<5 {
            s = go(s, "b", from: top(s, stack: tabA, root: "a"), dsl)
            s = go(s, "c", from: top(s, stack: tabA, root: "a"), dsl)
            XCTAssertEqual(s.paths[tabA]?.map(\.screen), ["b", "c"])
            s = go(s, "a", from: top(s, stack: tabA, root: "a"), dsl)
            XCTAssertEqual(s.paths[tabA], [], "A(根)へ戻ったら積まない")
            XCTAssertEqual(s.selectedTab, "a")
        }
    }

    func testPopToMiddleOfStack() throws {
        let dsl = try example("navigation")
        var s = NavigationState(selectedTab: "a")
        s = go(s, "b", from: Route(screen: "a", stack: tabA), dsl)
        s = go(s, "c", from: s.paths[tabA]!.last!, dsl)
        s = go(s, "b", from: s.paths[tabA]!.last!, dsl)
        XCTAssertEqual(s.paths[tabA]?.map(\.screen), ["b"], "スタックにある B へは pop")
    }

    func testJumpToOtherTabRootSwitchesTab() throws {
        let dsl = try example("navigation")
        var s = NavigationState(paths: [tabOther: [Route(screen: "b", stack: tabOther)]], selectedTab: "a")
        s = go(s, "b", from: Route(screen: "a", stack: tabA), dsl)
        s = go(s, "c", from: s.paths[tabA]!.last!, dsl)
        s = go(s, "other", from: s.paths[tabA]!.last!, dsl)
        XCTAssertEqual(s.selectedTab, "other")
        XCTAssertEqual(s.paths[tabOther], [], "切り替えたタブは根まで戻す")
        XCTAssertEqual(s.paths[tabA]?.map(\.screen), ["b", "c"], "元のタブのスタックはそのまま")
        // 別タブから A(タブの根)へ
        s = go(s, "a", from: Route(screen: "other", stack: tabOther), dsl)
        XCTAssertEqual(s.selectedTab, "a")
        XCTAssertEqual(s.paths[tabA], [])
    }

    func testSheetToTabRootClosesModal() throws {
        let dsl = try example("navigation")
        var s = NavigationState(selectedTab: "a")
        s = go(s, "c", from: Route(screen: "a", stack: tabA), dsl, presentation: .sheet)
        XCTAssertEqual(s.sheet?.screen, "c")
        s = go(s, "other", from: s.sheet!, dsl)
        XCTAssertNil(s.sheet, "モーダルを閉じてから切り替える")
        XCTAssertEqual(s.selectedTab, "other")

        s = go(s, "c", from: Route(screen: "other", stack: tabOther), dsl, presentation: .sheet)
        s = go(s, "b", from: s.sheet!, dsl)
        XCTAssertEqual(s.paths["sheet"]?.map(\.screen), ["b"], "下に無い画面はモーダル内に push")
        s = go(s, "a", from: s.paths["sheet"]!.last!, dsl)
        XCTAssertNil(s.sheet)
        XCTAssertNil(s.paths["sheet"])
        XCTAssertEqual(s.selectedTab, "a")
    }

    func testSheetToScreenInUnderlyingStackPops() throws {
        let dsl = try example("navigation")
        var s = NavigationState(paths: [tabA: [Route(screen: "b", stack: tabA), Route(screen: "c", stack: tabA)]], selectedTab: "a")
        s = go(s, "c", from: s.paths[tabA]!.last!, dsl, presentation: .sheet)
        s = go(s, "b", from: s.sheet!, dsl)
        XCTAssertNil(s.sheet)
        XCTAssertEqual(s.paths[tabA]?.map(\.screen), ["b"])
    }

    func testRecordIdDistinguishesPushedScreens() throws {
        let dsl = try example("kakeibo")
        let home = Route(screen: "home", stack: Route.tabStack("home"))
        var s = NavigationState(selectedTab: "home")
        s = go(s, "expense_form", from: home, dsl, recordId: "r1")
        s = go(s, "expense_form", from: s.paths[home.stack]!.last!, dsl, recordId: "r2")
        XCTAssertEqual(s.paths[home.stack]?.map(\.recordId), ["r1", "r2"], "別レコードは別物として push")
        s = go(s, "expense_form", from: s.paths[home.stack]!.last!, dsl, recordId: "r1")
        XCTAssertEqual(s.paths[home.stack]?.map(\.recordId), ["r1"], "同じレコードなら pop")
    }

    /// 既存例(家計簿・筋トレ・散歩)の遷移は変わらない: フォームへ push / sheet / fullScreen、保存で close。
    func testExistingExamplesUnchanged() throws {
        for (name, form) in [("kakeibo", "expense_form"), ("training", "workout_form")] {
            let dsl = try example(name)
            let home = Route(screen: "home", stack: Route.tabStack("home"))
            var s = NavigationState(selectedTab: "home")
            s = go(s, form, from: home, dsl)
            XCTAssertEqual(s.paths[home.stack], [Route(screen: form, stack: home.stack)], name)
            s = NavigationResolver.close(s, s.paths[home.stack]!.last!)
            XCTAssertEqual(s.paths[home.stack], [], name)
            s = go(s, form, from: home, dsl, presentation: .sheet)
            XCTAssertEqual(s.sheet, Route(screen: form, stack: "sheet"), name)
            XCTAssertEqual(s.selectedTab, "home", name)
            s = NavigationResolver.close(s, s.sheet!)
            XCTAssertNil(s.sheet, name)
        }
        let walk = try example("walk")
        let main = Route(screen: "home")
        var s = NavigationState()
        s = go(s, "memo_form", from: main, walk, presentation: .fullScreen)
        XCTAssertEqual(s.fullScreen, Route(screen: "memo_form", stack: "full"))
        s = NavigationResolver.close(s, s.fullScreen!)
        XCTAssertNil(s.fullScreen)
        s = go(s, "memo_form", from: main, walk)
        XCTAssertEqual(s.paths["main"], [Route(screen: "memo_form")])
        s = go(s, "home", from: s.paths["main"]!.last!, walk)
        XCTAssertEqual(s.paths["main"], [], "タブ無しでも根へは pop")
    }

    func testNewSheetStartsAtRoot() throws {
        let dsl = try example("navigation")
        var s = NavigationState(paths: ["sheet": [Route(screen: "b", stack: "sheet")]], selectedTab: "a")
        s = go(s, "c", from: Route(screen: "a", stack: tabA), dsl, presentation: .sheet)
        XCTAssertNil(s.paths["sheet"], "前のモーダルの積み残しを持ち越さない")
    }
}
