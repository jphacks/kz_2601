import XCTest
@testable import MetamonDSL

/// 手で動かす編集モード(#1442)のマス目への吸着・重なり・JSON Patch。
final class GridEditTests: XCTestCase {
    private func area(_ s: String) -> GridArea { GridArea(s)! }

    func testNotationRoundTrips() {
        for s in ["A1:C3", "D11:F12", "A1:F48", "B7:B7"] {
            XCTAssertEqual(area(s).notation, s)
        }
    }

    // MARK: - 吸着

    func testMoveSnapsToNearestCell() {
        let a = area("A1:B2")
        // 0.4 セルは動かない・0.6 セルは 1 つ動く
        XCTAssertEqual(GridEditMath.moved(a, dx: 40, dy: 20, cellW: 100, cellH: 50).notation, "A1:B2")
        XCTAssertEqual(GridEditMath.moved(a, dx: 60, dy: 30, cellW: 100, cellH: 50).notation, "B2:C3")
        XCTAssertEqual(GridEditMath.moved(a, dx: 210, dy: 260, cellW: 100, cellH: 50).notation, "C6:D7")
    }

    func testMoveStaysInsideGrid() {
        XCTAssertEqual(GridEditMath.moved(area("B2:C3"), dx: -1000, dy: -1000, cellW: 100, cellH: 50).notation, "A1:B2")
        XCTAssertEqual(GridEditMath.moved(area("B2:C3"), dx: 1000, dy: 0, cellW: 100, cellH: 50).notation, "E2:F3")
        XCTAssertEqual(GridEditMath.moved(area("A1:F2"), dx: 0, dy: 100_000, cellW: 100, cellH: 50).notation, "A47:F48")
    }

    func testResizeKeepsTopLeftAndMinimumOneCell() {
        XCTAssertEqual(GridEditMath.resized(area("B2:C3"), dx: 100, dy: 100, cellW: 100, cellH: 50).notation, "B2:D5")
        XCTAssertEqual(GridEditMath.resized(area("B2:C3"), dx: -1000, dy: -1000, cellW: 100, cellH: 50).notation, "B2:B2")
        XCTAssertEqual(GridEditMath.resized(area("B2:C3"), dx: 1000, dy: 100_000, cellW: 100, cellH: 50).notation, "B2:F48")
    }

    func testZeroCellSizeDoesNotMove() {
        XCTAssertEqual(GridEditMath.moved(area("B2:C3"), dx: 50, dy: 50, cellW: 0, cellH: 0).notation, "B2:C3")
        XCTAssertEqual(GridEditMath.resized(area("B2:C3"), dx: .nan, dy: .infinity, cellW: 10, cellH: 10).notation, "B2:C3")
    }

    // MARK: - 重なり

    private func screen() throws -> Screen {
        let json = """
        {"id":"home","title":"ホーム","components":[
          {"id":"summary","type":"text","text":"合計","area":"A1:F2"},
          {"id":"note","type":"text","text":"積む"},
          {"id":"recent","type":"text","text":"一覧","area":"A3:F10"},
          {"id":"add","type":"button","label":"追加","area":"D11:F12","action":{"type":"navigate","screen":"home"}}
        ]}
        """
        return try JSONDecoder().decode(Screen.self, from: Data(json.utf8))
    }

    func testDraftTakesOnlyPlacedComponents() throws {
        let d = GridEditDraft(screen: try screen(), screenIndex: 1)
        XCTAssertEqual(d.items.map(\.componentId), ["summary", "recent", "add"])
        XCTAssertEqual(d.items.map(\.index), [0, 2, 3])
        XCTAssertFalse(d.hasChanges)
        XCTAssertEqual(d.patch, [])
    }

    func testOverlapIsRefusedLikeBackend() throws {
        var d = GridEditDraft(screen: try screen(), screenIndex: 0)
        XCTAssertEqual(d.conflicts(for: "add", at: area("A10:C11")), ["recent"])
        XCTAssertFalse(d.place("add", at: area("A10:C11")))
        XCTAssertEqual(d.item("add")?.area.notation, "D11:F12")
        // 隣り合うだけ(セルを共有しない)は置ける
        XCTAssertTrue(d.place("add", at: area("A11:C12")))
        XCTAssertEqual(d.item("add")?.area.notation, "A11:C12")
        // 自分の元の場所とは重なってよい
        XCTAssertTrue(d.place("add", at: area("A11:D12")))
    }

    func testRemovedComponentNoLongerBlocks() throws {
        var d = GridEditDraft(screen: try screen(), screenIndex: 0)
        XCTAssertTrue(d.remove("recent"))
        XCTAssertTrue(d.conflicts(for: "add", at: area("A5:F12")).isEmpty)
        XCTAssertFalse(d.place("recent", at: area("A3:F4")), "消した部品は動かせない")
    }

    func testLastComponentCannotBeRemoved() throws {
        let json = #"{"id":"s","title":"","components":[{"type":"text","text":"x","area":"A1:F2"}]}"#
        var d = GridEditDraft(screen: try JSONDecoder().decode(Screen.self, from: Data(json.utf8)), screenIndex: 0)
        XCTAssertFalse(d.canRemove)
        XCTAssertFalse(d.remove(d.items[0].componentId))
    }

    // MARK: - JSON Patch

    func testPatchReplacesAreaAndRemovesFromTheBack() throws {
        var d = GridEditDraft(screen: try screen(), screenIndex: 1)
        XCTAssertTrue(d.remove("summary"))
        XCTAssertTrue(d.place("add", at: area("A11:C12")))
        XCTAssertTrue(d.remove("recent"))
        XCTAssertEqual(d.patch, [
            JSONPatchOp(op: "test", path: "/screens/1/id", value: .string("home")),
            JSONPatchOp(op: "test", path: "/screens/1/components/3/area", value: .string("D11:F12")),
            JSONPatchOp(op: "replace", path: "/screens/1/components/3/area", value: .string("A11:C12")),
            JSONPatchOp(op: "test", path: "/screens/1/components/2/area", value: .string("A3:F10")),
            JSONPatchOp(op: "remove", path: "/screens/1/components/2"),
            JSONPatchOp(op: "test", path: "/screens/1/components/0/area", value: .string("A1:F2")),
            JSONPatchOp(op: "remove", path: "/screens/1/components/0"),
        ])
    }

    func testMovingBackToOriginalIsNoChange() throws {
        var d = GridEditDraft(screen: try screen(), screenIndex: 0)
        XCTAssertTrue(d.place("add", at: area("A11:C12")))
        XCTAssertTrue(d.place("add", at: area("D11:F12")))
        XCTAssertFalse(d.hasChanges)
        XCTAssertEqual(d.patch, [])
    }

    func testPatchEncodesAsRFC6902() throws {
        let data = try JSONEncoder().encode([
            JSONPatchOp(op: "replace", path: "/screens/0/components/1/area", value: .string("A1:B2")),
            JSONPatchOp(op: "remove", path: "/screens/0/components/2"),
        ])
        let decoded = try JSONSerialization.jsonObject(with: data) as? [[String: String]]
        XCTAssertEqual(decoded, [
            ["op": "replace", "path": "/screens/0/components/1/area", "value": "A1:B2"],
            ["op": "remove", "path": "/screens/0/components/2"],
        ])
    }

    func testAppliedMatchesPatch() throws {
        var d = GridEditDraft(screen: try screen(), screenIndex: 0)
        XCTAssertTrue(d.place("add", at: area("A11:C12")))
        XCTAssertTrue(d.remove("summary"))
        let out = d.applied(to: try screen())
        XCTAssertEqual(out.components.map(\.id), ["note", "recent", "add"])
        XCTAssertEqual(out.components.map(\.area), [nil, "A3:F10", "A11:C12"])
        XCTAssertTrue(out.overlappingAreas.isEmpty)
    }
}
