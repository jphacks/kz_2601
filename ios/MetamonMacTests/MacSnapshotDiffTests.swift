import XCTest
@testable import Metamon

/// スナップショットの比べ方(#1136)。描画は使わないので CI でも回る
final class MacSnapshotDiffTests: XCTestCase {
    private func image(width: Int = 10, height: Int = 10, _ paint: (Int, Int) -> (UInt8, UInt8, UInt8)) -> CGImage {
        var bytes = [UInt8]()
        for y in 0..<height { for x in 0..<width { let (r, g, b) = paint(x, y); bytes += [r, g, b, 255] } }
        let provider = CGDataProvider(data: Data(bytes) as CFData)!
        return CGImage(width: width, height: height, bitsPerComponent: 8, bitsPerPixel: 32, bytesPerRow: width * 4,
                       space: CGColorSpace(name: CGColorSpace.sRGB)!, bitmapInfo: CGBitmapInfo(rawValue: CGImageAlphaInfo.premultipliedLast.rawValue),
                       provider: provider, decode: nil, shouldInterpolate: false, intent: .defaultIntent)!
    }

    func testSameImagePasses() {
        let a = image { _, _ in (200, 100, 50) }
        let diff = SnapshotDiff.compare(a, a, pixelTolerance: 32)
        XCTAssertEqual(diff.differing, 0)
        XCTAssertTrue(diff.passes(maxRatio: 0))
    }

    func testSmallColorShiftIsTolerated() {
        // 文字のアンチエイリアスの揺れくらいの差は数えない
        let a = image { _, _ in (200, 100, 50) }
        let b = image { _, _ in (220, 90, 60) }
        XCTAssertEqual(SnapshotDiff.compare(a, b, pixelTolerance: 32).differing, 0)
    }

    func testFewDifferingPixelsPassUnderRatio() {
        let a = image { _, _ in (255, 255, 255) }
        let b = image { x, y in x == 0 && y == 0 ? (0, 0, 0) : (255, 255, 255) }
        let diff = SnapshotDiff.compare(a, b, pixelTolerance: 32)
        XCTAssertEqual(diff.differing, 1)
        XCTAssertEqual(diff.ratio, 0.01, accuracy: 1e-9)
        XCTAssertTrue(diff.passes(maxRatio: 0.01))
        XCTAssertFalse(diff.passes(maxRatio: 0.005))
    }

    func testMovedBlockFails() {
        // 部品が 3 画素ずれた = 配置の崩れ
        let a = image { x, _ in x < 5 ? (0, 0, 0) : (255, 255, 255) }
        let b = image { x, _ in x < 8 ? (0, 0, 0) : (255, 255, 255) }
        XCTAssertFalse(SnapshotDiff.compare(a, b, pixelTolerance: 32).passes(maxRatio: 0.005))
    }

    func testSizeMismatchFails() {
        let a = image { _, _ in (0, 0, 0) }
        let b = image(width: 12) { _, _ in (0, 0, 0) }
        let diff = SnapshotDiff.compare(a, b, pixelTolerance: 255)
        XCTAssertTrue(diff.sizeMismatch)
        XCTAssertFalse(diff.passes(maxRatio: 1))
    }

    func testModeIsOffWithoutEnvironment() throws {
        let env = ProcessInfo.processInfo.environment
        try XCTSkipIf(env["METAMON_SNAPSHOTS"] != nil || env["METAMON_SNAPSHOT_RECORD"] != nil)
        XCTAssertEqual(SnapshotMode.current, .off, "CI の既定ではスナップショットを撮らない")
    }
}
