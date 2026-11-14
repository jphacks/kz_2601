import XCTest
import SwiftUI
@testable import Metamon

/// 表示メニューの 拡大 ⌘+ / 縮小 ⌘- / 実際のサイズ ⌘0 の上げ下げ(`MacTextZoom`・#2854)。
/// 1 段ずつの動きと端は MacAccessibilityStyleTests。ここは続けて押した時の通り道。
final class MacTextZoomTests: XCTestCase {
    /// ⌘+ を押し続けると 1 段ずつ上がり、上限で止まる
    func testZoomInWalksEveryStepToTheTop() {
        var step = MacTextZoom.defaultStep
        var sizes: [DynamicTypeSize] = [MacTextZoom.size(at: step)]
        while MacTextZoom.canZoomIn(step) {
            step = MacTextZoom.zoomedIn(step)
            sizes.append(MacTextZoom.size(at: step))
        }
        XCTAssertEqual(sizes, Array(MacTextZoom.steps[MacTextZoom.defaultStep...]))
        XCTAssertEqual(MacTextZoom.zoomedIn(step), step, "上限でもう一度押しても変わらない")
    }

    /// ⌘- を押し続けると 1 段ずつ下がり、下限で止まる
    func testZoomOutWalksEveryStepToTheBottom() {
        var step = MacTextZoom.defaultStep
        var sizes: [DynamicTypeSize] = [MacTextZoom.size(at: step)]
        while MacTextZoom.canZoomOut(step) {
            step = MacTextZoom.zoomedOut(step)
            sizes.append(MacTextZoom.size(at: step))
        }
        XCTAssertEqual(sizes, Array(MacTextZoom.steps[...MacTextZoom.defaultStep].reversed()))
        XCTAssertEqual(MacTextZoom.zoomedOut(step), step, "下限でもう一度押しても変わらない")
    }

    /// 上げてから同じ回数下げると元に戻る(端に当たらない範囲)
    func testZoomInThenOutReturns() {
        let start = MacTextZoom.defaultStep
        let up = MacTextZoom.zoomedIn(MacTextZoom.zoomedIn(start))
        XCTAssertEqual(MacTextZoom.zoomedOut(MacTextZoom.zoomedOut(up)), start)
    }

    /// ⌘0 は既定の段(.xLarge・100%)に戻す。既定の段では押せない。Mac の文字は小さいので system の既定(.large)より 1 段上
    func testActualSizeIsXLargeAt100Percent() {
        XCTAssertEqual(MacTextZoom.size(at: MacTextZoom.defaultStep), .xLarge)
        XCTAssertEqual(MacTextZoom.percent(at: MacTextZoom.defaultStep), 100)
        for step in MacTextZoom.steps.indices {
            XCTAssertEqual(MacTextZoom.isActualSize(step), step == MacTextZoom.defaultStep)
        }
    }

    /// 段が上がるほど本文の割合も大きい(設定の表示と逆にならない)
    func testPercentGrowsWithStep() {
        let percents = MacTextZoom.steps.indices.map(MacTextZoom.percent(at:))
        XCTAssertEqual(percents, percents.sorted())
        XCTAssertEqual(Set(percents).count, percents.count)
    }

    /// Mac の SwiftUI は dynamicTypeSize で文字が変わらないので、段の割合で拡大して描く。.large が等倍・既定はそれより大きい
    func testScaleFollowsBodyPoints() {
        let large = try! XCTUnwrap(MacTextZoom.steps.firstIndex(of: .large))
        XCTAssertEqual(MacTextZoom.scale(at: large), 1, accuracy: 0.0001)
        XCTAssertEqual(MacTextZoom.scale(at: MacTextZoom.defaultStep), 19.0 / 17.0, accuracy: 0.0001)
        let scales = MacTextZoom.steps.indices.map(MacTextZoom.scale(at:))
        XCTAssertEqual(scales, scales.sorted())
    }

    /// 上限は accessibility3・下限は xSmall(Mac のウインドウ幅で崩れない所まで)
    func testBounds() {
        XCTAssertEqual(MacTextZoom.steps.first, .xSmall)
        XCTAssertEqual(MacTextZoom.steps.last, .accessibility3)
    }
}
