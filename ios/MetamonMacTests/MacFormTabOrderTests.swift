import XCTest
import MetamonDSL
@testable import MetamonRenderer

/// Mac のフォームの Tab の順(#2065)。UI テスト(KeyboardShellUITests)は scheme で skipped なので、順番を決める所をここで確かめる。
@MainActor
final class MacFormTabOrderTests: XCTestCase {
    /// 入力の部品が混ざったフォーム。hstack・grid の中は DSL の並び(左 → 右)、見た目の位置(area)は順に効かない
    private func mixedForm() throws -> [Component] {
        let json = #"""
        [
          {"id": "title", "type": "textField", "field": "title", "area": "A3:F3"},
          {"id": "row", "type": "hstack", "children": [
            {"id": "amount", "type": "numberField", "field": "amount"},
            {"id": "date", "type": "datePicker", "field": "date"}
          ]},
          {"id": "memo", "type": "textField", "field": "memo", "multiline": true, "area": "A1:F2"},
          {"id": "heading", "type": "text", "text": "詳細"},
          {"id": "tiles", "type": "grid", "children": [
            {"id": "kind", "type": "picker", "field": "kind", "style": "segmented"},
            {"id": "done", "type": "toggle", "field": "done"},
            {"id": "at", "type": "timePicker", "field": "at"},
            {"id": "pin", "type": "secureField", "field": "pin"}
          ]},
          {"id": "body", "type": "textEditor", "field": "body"},
          {"id": "save", "type": "button", "label": "保存", "action": {"type": "saveRecord"}}
        ]
        """#
        return try JSONDecoder().decode([Component].self, from: Data(json.utf8))
    }

    private func ids(_ stops: [FormTabOrder.Stop]) -> [String] { stops.map(\.id) }

    func testStopsFollowDSLOrderDepthFirst() throws {
        let stops = FormTabOrder.stops(in: try mixedForm())
        XCTAssertEqual(ids(stops), ["title", "amount", "date", "memo", "kind", "done", "at", "pin", "body"])
        XCTAssertEqual(stops.map(\.kind), [.text, .text, .dateField, .text, .control, .control, .dateField, .text, .text])
    }

    func testFullKeyboardAccessOffSkipsControlsAndWraps() throws {
        let order = FormTabOrder.stops(in: try mixedForm())
        let mounted = Set(order.map(\.id))
        XCTAssertEqual(FormTabOrder.candidates(from: "memo", in: order, mounted: mounted, backward: false, fullKeyboardAccess: false),
                       ["at", "pin", "body", "title", "amount", "date"])
        // 最後の欄から先頭へ回る(見た目の順の先頭ではなく DSL の先頭)
        XCTAssertEqual(FormTabOrder.candidates(from: "body", in: order, mounted: mounted, backward: false, fullKeyboardAccess: false).first, "title")
        // Shift+Tab: 先頭から最後へ
        XCTAssertEqual(FormTabOrder.candidates(from: "title", in: order, mounted: mounted, backward: true, fullKeyboardAccess: false).first, "body")
    }

    func testFullKeyboardAccessOnStopsAtControlsAndLeavesAtTheEnd() throws {
        let order = FormTabOrder.stops(in: try mixedForm())
        let mounted = Set(order.map(\.id))
        XCTAssertEqual(FormTabOrder.candidates(from: "memo", in: order, mounted: mounted, backward: false, fullKeyboardAccess: true),
                       ["kind", "done", "at", "pin", "body"])
        XCTAssertEqual(FormTabOrder.candidates(from: "kind", in: order, mounted: mounted, backward: true, fullKeyboardAccess: true),
                       ["memo", "date", "amount", "title"])
        // 最後の欄・最初の欄の外はシステム(保存バーのボタンなど)に任せる
        XCTAssertEqual(FormTabOrder.candidates(from: "body", in: order, mounted: mounted, backward: false, fullKeyboardAccess: true), [])
        XCTAssertEqual(FormTabOrder.candidates(from: "title", in: order, mounted: mounted, backward: true, fullKeyboardAccess: true), [])
    }

    func testHiddenStopsAreSkipped() throws {
        let order = FormTabOrder.stops(in: try mixedForm())
        let mounted = Set(order.map(\.id)).subtracting(["amount", "date"]) // 条件で隠れた欄・閉じた disclosure の中
        XCTAssertEqual(FormTabOrder.candidates(from: "title", in: order, mounted: mounted, backward: false, fullKeyboardAccess: false).first, "memo")
    }

    func testUnknownOriginLeavesItToTheSystem() throws {
        let order = FormTabOrder.stops(in: try mixedForm())
        XCTAssertEqual(FormTabOrder.candidates(from: "nope", in: order, mounted: Set(order.map(\.id)), backward: false, fullKeyboardAccess: false), [])
        XCTAssertEqual(FormTabOrder.candidates(from: "title", in: [FormTabOrder.Stop(id: "title", kind: .text)], mounted: ["title"],
                                               backward: false, fullKeyboardAccess: false), [])
    }

    // MARK: 受け渡し役

    func testCoordinatorRequestsNextAndFallsThroughUnfocusableStop() async throws {
        let tabs = FormTabCoordinator()
        tabs.settleDelay = .milliseconds(300)
        tabs.order = FormTabOrder.stops(in: try mixedForm())
        for stop in tabs.order { tabs.mount(stop.id) }
        tabs.noteFocus("amount", true)

        XCTAssertTrue(tabs.move(from: "amount", backward: false, fullKeyboardAccess: false))
        XCTAssertEqual(tabs.request?.id, "date")
        // 日付の欄に止まれなかった(フォーカスが届かない) → 次の候補へ
        try await Task.sleep(for: .milliseconds(450))
        XCTAssertEqual(tabs.request?.id, "memo")
        tabs.noteFocus("amount", false)
        tabs.noteFocus("memo", true)
        try await Task.sleep(for: .milliseconds(450))
        XCTAssertEqual(tabs.request?.id, "memo", "止まれた欄の後に先へ進んでしまう")
    }

    func testCoordinatorDeclinesAtTheEndWithFullKeyboardAccess() throws {
        let tabs = FormTabCoordinator()
        tabs.order = FormTabOrder.stops(in: try mixedForm())
        for stop in tabs.order { tabs.mount(stop.id) }
        XCTAssertFalse(tabs.move(from: "body", backward: false, fullKeyboardAccess: true))
        XCTAssertNil(tabs.request)
    }

    func testUnmountCountsDuplicates() {
        let tabs = FormTabCoordinator()
        tabs.order = [.init(id: "a", kind: .text), .init(id: "b", kind: .text)]
        tabs.mount("a")
        tabs.mount("b")
        tabs.mount("b")
        tabs.unmount("b")
        XCTAssertTrue(tabs.move(from: "a", backward: false, fullKeyboardAccess: true))
        XCTAssertEqual(tabs.request?.id, "b")
        tabs.unmount("b")
        XCTAssertFalse(tabs.move(from: "a", backward: false, fullKeyboardAccess: true))
    }
}
