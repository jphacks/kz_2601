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
        XCTAssertEqual(FormTabOrder.candidates(from: "memo", in: order, backward: false, fullKeyboardAccess: false),
                       ["at", "pin", "body", "title", "amount", "date"])
        // 最後の欄から先頭へ回る(見た目の順の先頭ではなく DSL の先頭)
        XCTAssertEqual(FormTabOrder.candidates(from: "body", in: order, backward: false, fullKeyboardAccess: false).first, "title")
        // Shift+Tab: 先頭から最後へ
        XCTAssertEqual(FormTabOrder.candidates(from: "title", in: order, backward: true, fullKeyboardAccess: false).first, "body")
    }

    func testFullKeyboardAccessOnStopsAtControlsAndLeavesAtTheEnd() throws {
        let order = FormTabOrder.stops(in: try mixedForm())
        XCTAssertEqual(FormTabOrder.candidates(from: "memo", in: order, backward: false, fullKeyboardAccess: true),
                       ["kind", "done", "at", "pin", "body"])
        XCTAssertEqual(FormTabOrder.candidates(from: "kind", in: order, backward: true, fullKeyboardAccess: true),
                       ["memo", "date", "amount", "title"])
        // 最後の欄・最初の欄の外はシステム(保存バーのボタンなど)に任せる
        XCTAssertEqual(FormTabOrder.candidates(from: "body", in: order, backward: false, fullKeyboardAccess: true), [])
        XCTAssertEqual(FormTabOrder.candidates(from: "title", in: order, backward: true, fullKeyboardAccess: true), [])
    }

    func testCollapsedDisclosureHidesItsStopsFromTheDSL() throws {
        let json = #"""
        [
          {"id": "title", "type": "textField", "field": "title"},
          {"id": "more", "type": "disclosure", "title": "詳細", "children": [
            {"id": "note", "type": "textField", "field": "note"}
          ]},
          {"id": "open", "type": "disclosure", "expanded": true, "children": [
            {"id": "tag", "type": "textField", "field": "tag"}
          ]},
          {"id": "last", "type": "textField", "field": "last"}
        ]
        """#
        let layout = FormTabOrder.layout(in: try JSONDecoder().decode([Component].self, from: Data(json.utf8)))
        XCTAssertEqual(layout.stops.first { $0.id == "note" }?.disclosures, ["more"])
        XCTAssertEqual(layout.expandedByDefault, ["more": false, "open": true])

        let tabs = FormTabCoordinator()
        tabs.apply(layout)
        // 出ているかどうかではなく DSL の開閉で決める(閉じた disclosure の中は飛ばす)
        XCTAssertEqual(FormTabOrder.candidates(from: "title", in: tabs.order, collapsed: tabs.collapsed,
                                               backward: false, fullKeyboardAccess: false), ["tag", "last"])
        tabs.noteDisclosure("more", expanded: true)
        XCTAssertEqual(FormTabOrder.candidates(from: "title", in: tabs.order, collapsed: tabs.collapsed,
                                               backward: false, fullKeyboardAccess: false), ["note", "tag", "last"])
    }

    func testIncludeDropsStopsTheRendererDoesNotDraw() throws {
        // 隠す field(#262)などは include で落とす。出ていないだけの欄とは区別する
        let stops = FormTabOrder.stops(in: try mixedForm()) { !["amount", "row"].contains($0.id) }
        XCTAssertEqual(ids(stops), ["title", "memo", "kind", "done", "at", "pin", "body"])
    }

    func testLazyContainersGiveAScrollAnchor() throws {
        let json = #"""
        [
          {"id": "tiles", "type": "grid", "children": [
            {"id": "cell", "type": "vstack", "children": [{"id": "a", "type": "textField", "field": "a"}]},
            {"id": "b", "type": "textField", "field": "b"}
          ]},
          {"id": "cards", "type": "carousel", "children": [{"id": "c", "type": "textField", "field": "c"}]},
          {"id": "d", "type": "textField", "field": "d"}
        ]
        """#
        let stops = FormTabOrder.stops(in: try JSONDecoder().decode([Component].self, from: Data(json.utf8)))
        XCTAssertEqual(stops.map(\.anchor), ["cell", "b", "cards", nil])
    }

    func testUnknownOriginLeavesItToTheSystem() throws {
        let order = FormTabOrder.stops(in: try mixedForm())
        XCTAssertEqual(FormTabOrder.candidates(from: "nope", in: order, backward: false, fullKeyboardAccess: false), [])
        XCTAssertEqual(FormTabOrder.candidates(from: "title", in: [FormTabOrder.Stop(id: "title", kind: .text)],
                                               backward: false, fullKeyboardAccess: false), [])
    }

    // MARK: 受け渡し役

    /// 待ちを手で進める時計(実時間で眠らない)
    @MainActor
    private final class ManualClock {
        private var waiters: [CheckedContinuation<Void, Never>] = []
        var count: Int { waiters.count }
        func wait(_: Duration) async { await withCheckedContinuation { waiters.append($0) } }
        /// 一番古い待ちを終わらせる(待ちに入るまでは譲って待つ)
        func fireFirst() async {
            while waiters.isEmpty { await Task.yield() }
            waiters.removeFirst().resume()
        }
    }

    private func coordinator(clock: ManualClock) throws -> FormTabCoordinator {
        let tabs = FormTabCoordinator()
        tabs.wait = { await clock.wait($0) }
        tabs.apply(FormTabOrder.layout(in: try mixedForm()))
        for stop in tabs.order { tabs.mount(stop.id) }
        return tabs
    }

    /// 次に待ちが終わるのを待つ
    private func settled(_ tabs: FormTabCoordinator, count: Int = 1) -> XCTestExpectation {
        let e = expectation(description: "settled")
        e.expectedFulfillmentCount = count
        tabs.didSettle = { e.fulfill() }
        return e
    }

    func testCoordinatorFallsThroughUnfocusableStop() async throws {
        let clock = ManualClock()
        let tabs = try coordinator(clock: clock)
        tabs.noteFocus("amount", true)

        XCTAssertTrue(tabs.move(from: "amount", backward: false, fullKeyboardAccess: false))
        XCTAssertEqual(tabs.request?.id, "date")
        // 日付の欄に止まれなかった(フォーカスが届かない) → 次の候補へ
        let e = settled(tabs)
        await clock.fireFirst()
        await fulfillment(of: [e], timeout: 2)
        XCTAssertEqual(tabs.request?.id, "memo")
    }

    func testSuccessIsRecordedWhenFocusArrivesNotAtTheTimeout() async throws {
        let clock = ManualClock()
        let tabs = try coordinator(clock: clock)
        tabs.noteFocus("amount", true)
        XCTAssertTrue(tabs.move(from: "amount", backward: false, fullKeyboardAccess: false))
        // 待ちの途中で止まれた(止まった後にすぐ外れても、待ちの後で先へ進めない)
        tabs.noteFocus("date", true)
        XCTAssertNil(tabs.request)
        tabs.noteFocus("date", false)
        let e = settled(tabs)
        await clock.fireFirst()
        await fulfillment(of: [e], timeout: 2)
        XCTAssertNil(tabs.request, "止まれた欄の後に先へ進んでしまう")
    }

    func testUserMovingElsewhereCancelsTheHandOff() async throws {
        let clock = ManualClock()
        let tabs = try coordinator(clock: clock)
        XCTAssertTrue(tabs.move(from: "amount", backward: false, fullKeyboardAccess: false))
        tabs.noteFocus("title", true) // クリックで別の欄へ
        XCTAssertNil(tabs.request)
        let e = settled(tabs)
        await clock.fireFirst()
        await fulfillment(of: [e], timeout: 2)
        XCTAssertNil(tabs.request)
    }

    func testRepeatedTabCountsFromThePendingTargetAndCancelsTheOldWait() async throws {
        let clock = ManualClock()
        let tabs = try coordinator(clock: clock)
        XCTAssertTrue(tabs.move(from: "amount", backward: false, fullKeyboardAccess: false))
        XCTAssertEqual(tabs.request?.id, "date")
        // まだ amount にいる間にもう一度 Tab: date の次から数える
        XCTAssertTrue(tabs.move(from: "amount", backward: false, fullKeyboardAccess: false))
        XCTAssertEqual(tabs.request?.id, "memo")
        // 古い待ちが終わっても先へ進めない
        let e = settled(tabs)
        await clock.fireFirst()
        await fulfillment(of: [e], timeout: 2)
        XCTAssertEqual(tabs.request?.id, "memo")
    }

    func testUnmountedLazyStopIsRevealedBeforeFocus() throws {
        let json = #"""
        [
          {"id": "a", "type": "textField", "field": "a"},
          {"id": "tiles", "type": "grid", "children": [{"id": "b", "type": "textField", "field": "b"}]}
        ]
        """#
        let tabs = FormTabCoordinator()
        tabs.wait = { _ in try await Task.sleep(for: .seconds(60)) }
        tabs.apply(FormTabOrder.layout(in: try JSONDecoder().decode([Component].self, from: Data(json.utf8))))
        tabs.mount("a") // b は LazyVGrid でまだ出ていない
        XCTAssertTrue(tabs.move(from: "a", backward: false, fullKeyboardAccess: false))
        XCTAssertEqual(tabs.request?.id, "b")
        XCTAssertEqual(tabs.reveal?.id, "b")
    }

    func testCoordinatorDeclinesAtTheEndWithFullKeyboardAccess() throws {
        let tabs = FormTabCoordinator()
        tabs.apply(FormTabOrder.layout(in: try mixedForm()))
        for stop in tabs.order { tabs.mount(stop.id) }
        XCTAssertFalse(tabs.move(from: "body", backward: false, fullKeyboardAccess: true))
        XCTAssertNil(tabs.request)
    }

    func testTextEditorAtTheEndStepsOutInsteadOfInsertingTab() throws {
        let tabs = FormTabCoordinator()
        var steps: [Bool] = []
        tabs.systemStep = { steps.append($0) }
        tabs.apply(FormTabOrder.layout(in: try mixedForm()))
        for stop in tabs.order { tabs.mount(stop.id) }
        // 複数行の欄は断ると Tab の文字が入るので、システムの次へ進めて引き受ける
        XCTAssertTrue(tabs.move(from: "body", backward: false, fullKeyboardAccess: true, insertsTab: true))
        XCTAssertTrue(tabs.move(from: "nope", backward: true, fullKeyboardAccess: false, insertsTab: true))
        XCTAssertEqual(steps, [false, true])
        XCTAssertNil(tabs.request)
    }

    func testUnmountCountsDuplicates() {
        let tabs = FormTabCoordinator()
        tabs.wait = { _ in try await Task.sleep(for: .seconds(60)) }
        tabs.apply(.init(stops: [.init(id: "a", kind: .text), .init(id: "b", kind: .text, anchor: "cell")]))
        tabs.mount("a")
        tabs.mount("b")
        tabs.mount("b")
        tabs.unmount("b")
        XCTAssertTrue(tabs.move(from: "a", backward: false, fullKeyboardAccess: true))
        XCTAssertNil(tabs.reveal, "まだ 1 か所に出ている")
        tabs.noteFocus("b", true) // 止まれた
        tabs.unmount("b")
        XCTAssertTrue(tabs.move(from: "a", backward: false, fullKeyboardAccess: true))
        XCTAssertEqual(tabs.reveal?.id, "cell")
    }
}
