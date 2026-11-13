import XCTest
import MetamonDSL
@testable import MetamonRenderer
@testable import Metamon

/// #2244: 画面マップの文言(ScreenMapCopy)・矢印の縁の計算(ArrowGeometry.edgePoint)・カードの置き場所(ScreenMapLayout.frame)。描画(drawArrows)は見ない。
@MainActor
final class ScreenMapLayoutTests: XCTestCase {
    private var saved: AppLanguage = .system

    override func setUp() {
        saved = LanguagePreference.shared.setting
        LanguagePreference.shared.setting = .ja
    }

    override func tearDown() { LanguagePreference.shared.setting = saved }

    private func edge(_ t: ScreenGraph.Trigger, _ p: Presentation = .push) -> ScreenGraph.Edge {
        ScreenGraph.Edge(from: "a", to: "b", presentation: p, trigger: t)
    }

    // MARK: 文言

    func testContextPrefixAndChipWithoutComponent() {
        let s = ScreenContext(id: "home", title: "ホーム", component: nil)
        XCTAssertEqual(ScreenMapCopy.contextPrefix(s), "画面「ホーム」(id: home)で: ")
        XCTAssertEqual(ScreenMapCopy.chipText(s), "画面「ホーム」について話す")
    }

    func testContextWithComponent() {
        let s = ScreenContext(id: "home", title: "ホーム", component: AIButtonComponent(id: "c1", label: "追加"))
        XCTAssertEqual(ScreenMapCopy.contextPrefix(s), "画面「ホーム」(id: home)の部品「追加」(id: c1)で: ")
        XCTAssertEqual(ScreenMapCopy.chipText(s), "画面「ホーム」の「追加」について話す")
    }

    func testEmptyTitleIsPassedThroughAsIs() {
        // 画面の名前の補いは呼び出し側(ScreenGraph.Node は空なら id)。文言は渡された名前をそのまま入れる
        let s = ScreenContext(id: "x", title: "", component: nil)
        XCTAssertEqual(ScreenMapCopy.chipText(s), "画面「」について話す")
        XCTAssertEqual(ScreenMapCopy.contextPrefix(s), "画面「」(id: x)で: ")
    }

    func testLongTitleIsNotTruncated() {
        let long = String(repeating: "長", count: 200)
        let s = ScreenContext(id: "x", title: long, component: nil)
        XCTAssertEqual(ScreenMapCopy.chipText(s), "画面「\(long)」について話す")
    }

    func testDescribeEveryTriggerJapanese() {
        XCTAssertEqual(ScreenMapCopy.describe(edge(.button("追加"))), "ボタン「追加」")
        XCTAssertEqual(ScreenMapCopy.describe(edge(.toolbar("設定"))), "上部の「設定」")
        XCTAssertEqual(ScreenMapCopy.describe(edge(.itemTap)), "一覧の行")
        XCTAssertEqual(ScreenMapCopy.describe(edge(.dateTap)), "日付のタップ")
        XCTAssertEqual(ScreenMapCopy.describe(edge(.afterSave)), "保存したあと")
        XCTAssertEqual(ScreenMapCopy.describe(edge(.other)), "部品のタップ")
    }

    func testDescribePresentationSuffix() {
        XCTAssertEqual(ScreenMapCopy.describe(edge(.itemTap, .push)), "一覧の行")
        XCTAssertEqual(ScreenMapCopy.describe(edge(.itemTap, .sheet)), "一覧の行(シート)")
        XCTAssertEqual(ScreenMapCopy.describe(edge(.button("追加"), .fullScreen)), "ボタン「追加」(全画面)")
    }

    func testEnglish() {
        LanguagePreference.shared.setting = .en
        let s = ScreenContext(id: "home", title: "Home", component: AIButtonComponent(id: "c1", label: "Add"))
        XCTAssertEqual(ScreenMapCopy.contextPrefix(s), "On screen \"Home\" (id: home), component \"Add\" (id: c1): ")
        XCTAssertEqual(ScreenMapCopy.chipText(s), "Talking about \"Add\" on \"Home\"")
        XCTAssertEqual(ScreenMapCopy.chipText(ScreenContext(id: "home", title: "Home", component: nil)), "Talking about \"Home\"")
        XCTAssertEqual(ScreenMapCopy.describe(edge(.button("Add"), .sheet)), "button \"Add\" (sheet)")
        XCTAssertEqual(ScreenMapCopy.describe(edge(.toolbar("Edit"), .fullScreen)), "top bar \"Edit\" (full screen)")
        XCTAssertEqual(ScreenMapCopy.describe(edge(.itemTap)), "list row")
        XCTAssertEqual(ScreenMapCopy.describe(edge(.dateTap)), "date tap")
        XCTAssertEqual(ScreenMapCopy.describe(edge(.afterSave)), "after saving")
        XCTAssertEqual(ScreenMapCopy.describe(edge(.other)), "component tap")
    }

    // MARK: edgePoint(矩形 0,0 100x40 → 中心 (50,20)・縁の少し外側 +3)

    private let rect = CGRect(x: 0, y: 0, width: 100, height: 40)

    private func assertPoint(_ p: CGPoint, _ x: CGFloat, _ y: CGFloat, file: StaticString = #filePath, line: UInt = #line) {
        XCTAssertEqual(p.x, x, accuracy: 0.001, file: file, line: line)
        XCTAssertEqual(p.y, y, accuracy: 0.001, file: file, line: line)
    }

    func testEdgePointFourSides() {
        assertPoint(ArrowGeometry.edgePoint(of: rect, toward: CGPoint(x: 300, y: 20)), 103, 20)
        assertPoint(ArrowGeometry.edgePoint(of: rect, toward: CGPoint(x: -200, y: 20)), -3, 20)
        assertPoint(ArrowGeometry.edgePoint(of: rect, toward: CGPoint(x: 50, y: -100)), 50, -3)
        assertPoint(ArrowGeometry.edgePoint(of: rect, toward: CGPoint(x: 50, y: 200)), 50, 43)
    }

    func testEdgePointSameAsCenterReturnsCenter() {
        assertPoint(ArrowGeometry.edgePoint(of: rect, toward: CGPoint(x: 50, y: 20)), 50, 20)
    }

    func testEdgePointDiagonalHitsNearerSide() {
        // 右下 45 度: 高さのほうが先に縁に着く(下の縁 y = 43)
        assertPoint(ArrowGeometry.edgePoint(of: rect, toward: CGPoint(x: 150, y: 120)), 73, 43)
        // 右へ浅い角度: 幅のほうが先(右の縁 x = 103)
        assertPoint(ArrowGeometry.edgePoint(of: rect, toward: CGPoint(x: 350, y: 10)), 103, 20 - 10 * 53 / 300)
    }

    // MARK: frame(of:)

    private func graph(_ extra: String = "") throws -> ScreenGraph {
        let json = """
        {"schemaVersion":1,"app":{"name":"t"},"initialScreen":"home","entities":[],"screens":[
          {"id":"home","title":"ホーム","components":[
            {"type":"button","label":"A","action":{"type":"navigate","screen":"a"}},
            {"type":"button","label":"B","action":{"type":"navigate","screen":"b"}}]},
          {"id":"a","title":"A"},{"id":"b","title":"B"}\(extra)
        ]}
        """
        return ScreenGraph(dsl: try JSONDecoder().decode(AppDSL.self, from: Data(json.utf8)))
    }

    private func layout(_ g: ScreenGraph, width: CGFloat) -> ScreenMapLayout {
        var l = ScreenMapLayout(graph: g, availableWidth: width)
        l.inset = 16
        return l
    }

    func testFrameCentersEachRowAndStepsByRowHeight() throws {
        let g = try graph()
        XCTAssertEqual(g.rows, [["home"], ["a", "b"]])
        let l = layout(g, width: 400)
        XCTAssertEqual(l.cardWidth, 150, "広くても最大幅")
        XCTAssertEqual(l.frame(of: g.node("home")!), CGRect(x: 125, y: 16, width: 150, height: 66))
        // 2 枚の段: 幅 318・左 41・次の列は 168 右
        XCTAssertEqual(l.frame(of: g.node("a")!), CGRect(x: 41, y: 146, width: 150, height: 66))
        XCTAssertEqual(l.frame(of: g.node("b")!), CGRect(x: 209, y: 146, width: 150, height: 66))
        XCTAssertEqual(l.contentHeight, 16 * 2 + 66 * 2 + 64)
    }

    func testNarrowWidthKeepsMinCardWidthAndGrowsContent() throws {
        let g = try graph()
        let l = layout(g, width: 200)
        XCTAssertEqual(l.cardWidth, 104, "収まらなければ最小幅")
        XCTAssertEqual(l.contentWidth, 16 * 2 + 2 * 104 + 18, "横スクロールの幅")
        XCTAssertEqual(l.frame(of: g.node("a")!).minX, 16, accuracy: 0.001, "いちばん広い段は左の余白から")
    }

    func testUnreachableRowIsPushedDownByLabel() throws {
        let g = try graph(#",{"id":"lost","title":"迷子"}"#)
        XCTAssertEqual(g.rows.last, ["lost"])
        let l = layout(g, width: 400)
        XCTAssertEqual(l.frame(of: g.node("lost")!).minY, 16 + 2 * (66 + 64) + 26)
        XCTAssertEqual(l.frame(of: g.node("a")!).minY, 146, "到達できる段は動かない")
    }
}
