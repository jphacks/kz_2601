import XCTest
import MetamonDSL
@testable import MetamonRenderer

/// 見た目監査(#131)の回帰: フォーム画面の段組み・chart の x 軸ラベル短縮。
final class VisualAuditLayoutTests: XCTestCase {
    private func screen(_ json: String) throws -> Screen {
        try JSONDecoder().decode(Screen.self, from: Data(json.utf8))
    }

    func testFormBandsGroupOverlappingRowsAndKeepOrder() throws {
        // AI がよく出す形: 入力 section(小さすぎる A1:F4)+ 横並び 2 ボタン + 最下段の削除ボタン(順不同で来る)
        let s = try screen(#"""
        {"id":"f","title":"f","entity":"e","components":[
          {"id":"del","type":"button","label":"削除","area":"A11:F12"},
          {"id":"sec","type":"section","area":"A1:F4","children":[]},
          {"id":"r","type":"button","label":"右","area":"D5:F6"},
          {"id":"l","type":"button","label":"左","area":"A5:C6"}
        ]}
        """#)
        let bands = FormFlowLayout.bands(s.gridPlacement.placed)
        XCTAssertEqual(bands.map { $0.items.map(\.component.id) }, [["sec"], ["l", "r"], ["del"]])
        XCTAssertEqual(bands.last?.row1, 11)
    }

    func testFormBandsMergeTallSideBySide() throws {
        let s = try screen(#"""
        {"id":"f","title":"f","entity":"e","components":[
          {"id":"a","type":"section","area":"A1:C6","children":[]},
          {"id":"b","type":"section","area":"D3:F8","children":[]},
          {"id":"c","type":"text","text":"x","area":"A9:F9"}
        ]}
        """#)
        let bands = FormFlowLayout.bands(s.gridPlacement.placed)
        XCTAssertEqual(bands.map { $0.items.map(\.component.id) }, [["a", "b"], ["c"]])
    }

    func testChartAxisLabelShortensLongCategory() {
        XCTAssertEqual(ChartLeaf.axisLabel("食費"), "食費")
        XCTAssertEqual(ChartLeaf.axisLabel("10/24"), "10/24")
        XCTAssertEqual(ChartLeaf.axisLabel("駅前の新しくできたイタリアン"), "駅前の新し…")
    }
}
