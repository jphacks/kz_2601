import XCTest
import MetamonDSL
@testable import MetamonRenderer

/// #2847: Mac のフォームの Tab の順(#2065)の純粋関数 `FormTabOrder`。
final class FormTabOrderTests: XCTestCase {
    private func components(_ json: String) throws -> [Component] {
        try JSONDecoder().decode([Component].self, from: Data(json.utf8))
    }

    /// 横並び・折りたたみ・grid・carousel を含む画面
    private let screen = #"""
    [
      {"id":"title","type":"text","text":"見出し"},
      {"id":"row","type":"hstack","children":[
        {"id":"name","type":"textField","field":"name"},
        {"id":"done","type":"toggle","field":"done"}]},
      {"id":"more","type":"disclosure","title":"詳しく","children":[
        {"id":"memo","type":"textEditor","field":"memo"},
        {"id":"inner","type":"disclosure","title":"さらに","expanded":true,"children":[
          {"id":"due","type":"datePicker","field":"due"}]}]},
      {"id":"g","type":"grid","children":[
        {"id":"cell","type":"vstack","children":[{"id":"qty","type":"numberField","field":"qty"}]},
        {"id":"at","type":"timePicker","field":"at"}]},
      {"id":"car","type":"carousel","children":[
        {"id":"pin","type":"secureField","field":"pin"},
        {"id":"kind","type":"picker","field":"kind"}]},
      {"id":"go","type":"button","label":"保存"}
    ]
    """#

    // MARK: kind

    func testKindOfInputTypes() {
        for t: ComponentType in [.textField, .textEditor, .numberField, .secureField] {
            XCTAssertEqual(FormTabOrder.kind(of: t), .text, "\(t)")
        }
        XCTAssertEqual(FormTabOrder.kind(of: .datePicker), .dateField)
        XCTAssertEqual(FormTabOrder.kind(of: .timePicker), .dateField)
        XCTAssertEqual(FormTabOrder.kind(of: .toggle), .control)
        XCTAssertEqual(FormTabOrder.kind(of: .picker), .control)
        for t: ComponentType in [.text, .button, .disclosure, .hstack, .slider, .photoField] {
            XCTAssertNil(FormTabOrder.kind(of: t), "\(t)")
        }
    }

    // MARK: stops

    func testStopsPickOnlyInputsInDSLOrder() throws {
        let stops = FormTabOrder.stops(in: try components(screen))
        XCTAssertEqual(stops.map(\.id), ["name", "done", "memo", "due", "qty", "at", "pin", "kind"],
                       "親 → 子の深さ優先。text・button・入れ物は止まらない")
        XCTAssertEqual(stops.map(\.kind), [.text, .control, .text, .dateField, .text, .dateField, .text, .control])
    }

    func testStopsSkipExcludedComponentAndItsChildren() throws {
        let stops = FormTabOrder.stops(in: try components(screen)) { $0.id != "more" && $0.id != "pin" }
        XCTAssertEqual(stops.map(\.id), ["name", "done", "qty", "at", "kind"], "除いた折りたたみは中ごと出さない")
    }

    func testStopsEmpty() throws {
        XCTAssertEqual(FormTabOrder.stops(in: []), [])
        XCTAssertEqual(FormTabOrder.stops(in: try components(#"[{"id":"t","type":"text","text":"x"}]"#)), [])
    }

    // MARK: layout

    func testLayoutMarksDisclosuresAndAnchors() throws {
        let layout = FormTabOrder.layout(in: try components(screen))
        let byId = Dictionary(uniqueKeysWithValues: layout.stops.map { ($0.id, $0) })
        XCTAssertEqual(byId["name"]?.disclosures, [])
        XCTAssertEqual(byId["memo"]?.disclosures, ["more"])
        XCTAssertEqual(byId["due"]?.disclosures, ["more", "inner"], "入れ子の折りたたみは外 → 内の順")
        XCTAssertEqual(layout.expandedByDefault, ["more": false, "inner": true], "expanded が無ければ閉じている")

        XCTAssertNil(byId["name"]?.anchor)
        XCTAssertNil(byId["memo"]?.anchor)
        XCTAssertEqual(byId["qty"]?.anchor, "cell", "grid の中は grid の子(セル)へスクロール")
        XCTAssertEqual(byId["at"]?.anchor, "at")
        XCTAssertEqual(byId["pin"]?.anchor, "car", "carousel の中は carousel まで")
        XCTAssertEqual(byId["kind"]?.anchor, "car")
    }

    func testLayoutCarouselInsideGridKeepsOuterAnchor() throws {
        let layout = FormTabOrder.layout(in: try components(#"""
        [{"id":"g","type":"grid","children":[{"id":"cell","type":"vstack","children":[
          {"id":"car","type":"carousel","children":[{"id":"f","type":"textField","field":"f"}]}]}]}]
        """#))
        XCTAssertEqual(layout.stops.first?.anchor, "cell", "外の grid のセルへのスクロールを引き継ぐ")
    }

    // MARK: candidates

    private func order() throws -> [FormTabOrder.Stop] {
        FormTabOrder.stops(in: try components(screen))
    }

    func testForwardWithoutFullKeyboardAccessSkipsControlsAndWraps() throws {
        let o = try order()
        XCTAssertEqual(FormTabOrder.candidates(from: "name", in: o, backward: false, fullKeyboardAccess: false),
                       ["memo", "due", "qty", "at", "pin"], "スイッチ・選択肢は飛ばす。自分は候補に入らない")
        XCTAssertEqual(FormTabOrder.candidates(from: "pin", in: o, backward: false, fullKeyboardAccess: false),
                       ["name", "memo", "due", "qty", "at"], "端を越えたら先頭へ回る")
    }

    func testBackwardWithoutFullKeyboardAccessWraps() throws {
        let o = try order()
        XCTAssertEqual(FormTabOrder.candidates(from: "qty", in: o, backward: true, fullKeyboardAccess: false),
                       ["due", "memo", "name", "pin", "at"], "近い順に戻り、先頭を越えたら末尾から")
        XCTAssertEqual(FormTabOrder.candidates(from: "name", in: o, backward: true, fullKeyboardAccess: false),
                       ["pin", "at", "qty", "due", "memo"])
    }

    func testFullKeyboardAccessIncludesControlsAndStopsAtEdge() throws {
        let o = try order()
        XCTAssertEqual(FormTabOrder.candidates(from: "name", in: o, backward: false, fullKeyboardAccess: true),
                       ["done", "memo", "due", "qty", "at", "pin", "kind"])
        XCTAssertEqual(FormTabOrder.candidates(from: "kind", in: o, backward: false, fullKeyboardAccess: true), [],
                       "on の時は端で終わり(システムが保存バーへ進める)")
        XCTAssertEqual(FormTabOrder.candidates(from: "name", in: o, backward: true, fullKeyboardAccess: true), [])
        XCTAssertEqual(FormTabOrder.candidates(from: "at", in: o, backward: true, fullKeyboardAccess: true),
                       ["qty", "due", "memo", "done", "name"])
    }

    func testCollapsedDisclosureIsSkipped() throws {
        let o = try order()
        XCTAssertEqual(FormTabOrder.candidates(from: "name", in: o, collapsed: ["more"], backward: false, fullKeyboardAccess: false),
                       ["qty", "at", "pin"], "閉じた折りたたみの中(入れ子も)を飛ばす")
        XCTAssertEqual(FormTabOrder.candidates(from: "name", in: o, collapsed: ["inner"], backward: false, fullKeyboardAccess: false),
                       ["memo", "qty", "at", "pin"], "内側だけ閉じていれば外の欄には止まる")
    }

    func testOriginInsideCollapsedDisclosureStillMoves() throws {
        let o = try order()
        XCTAssertEqual(FormTabOrder.candidates(from: "memo", in: o, collapsed: ["more"], backward: false, fullKeyboardAccess: true),
                       ["qty", "at", "pin", "kind"], "今いる欄は隠れていても起点になる")
    }

    func testOriginControlWithoutFullKeyboardAccessStillMoves() throws {
        let o = try order()
        XCTAssertEqual(FormTabOrder.candidates(from: "done", in: o, backward: false, fullKeyboardAccess: false),
                       ["memo", "due", "qty", "at", "pin", "name"])
    }

    func testUnknownOriginGivesNoCandidates() throws {
        XCTAssertEqual(FormTabOrder.candidates(from: "nope", in: try order(), backward: false, fullKeyboardAccess: false), [])
        XCTAssertEqual(FormTabOrder.candidates(from: "a", in: [], backward: false, fullKeyboardAccess: false), [])
    }

    func testDuplicateIdsCountOnce() {
        let o: [FormTabOrder.Stop] = [.init(id: "a", kind: .text), .init(id: "b", kind: .text),
                                      .init(id: "a", kind: .text), .init(id: "c", kind: .text)]
        XCTAssertEqual(FormTabOrder.candidates(from: "a", in: o, backward: false, fullKeyboardAccess: false), ["b", "c"],
                       "同じ id が 2 か所にあっても最初の位置で 1 回だけ")
        XCTAssertEqual(FormTabOrder.candidates(from: "c", in: o, backward: false, fullKeyboardAccess: false), ["a", "b"])
    }

    func testSingleStopHasNoCandidates() {
        let o: [FormTabOrder.Stop] = [.init(id: "only", kind: .text)]
        XCTAssertEqual(FormTabOrder.candidates(from: "only", in: o, backward: false, fullKeyboardAccess: false), [])
        XCTAssertEqual(FormTabOrder.candidates(from: "only", in: o, backward: true, fullKeyboardAccess: false), [])
    }

    // MARK: contains

    func testContainsSearchesDescendants() throws {
        let cs = try components(screen)
        let more = try XCTUnwrap(cs.first { $0.id == "more" })
        XCTAssertTrue(FormTabOrder.contains("more", in: more))
        XCTAssertTrue(FormTabOrder.contains("due", in: more))
        XCTAssertFalse(FormTabOrder.contains("name", in: more))
    }
}
