import XCTest
import AppKit
import MetamonDSL
import MetamonServices
@testable import Metamon

/// フィードバックに添えるものの整え方(#983・#1330)。封筒から同じ版の DSL だけを整形し、スクショは上限の画素に収める
@MainActor
final class MacFeedbackContextTests: XCTestCase {
    // MARK: - prettyDSL(封筒 → 同じ版の DSL の全文)

    private func envelope(version: Int, dsl: String) throws -> Data {
        try XCTUnwrap(DocumentEnvelope.make(appId: "mac-live", version: version, dsl: Data(dsl.utf8)))
    }

    func testPrettyDSLReturnsOnlyDSLSortedAndUnescaped() throws {
        let json = MacFeedbackContext.prettyDSL(
            try envelope(version: 4, dsl: #"{"name":"家計簿","b":1,"a":2,"url":"https://tailor-it.app/s/abc"}"#),
            version: 4)
        let out = try XCTUnwrap(json)
        XCTAssertTrue(out.contains("\"name\" : \"家計簿\""), "整形して見せる")
        XCTAssertFalse(out.contains("\"appId\""), "封筒の外側(appId・version)は入れない")
        XCTAssertLessThan(try XCTUnwrap(out.range(of: "\"a\"")).lowerBound,
                          try XCTUnwrap(out.range(of: "\"b\"")).lowerBound, "鍵は並べ替える")
        XCTAssertTrue(out.contains("https://tailor-it.app/s/abc"), "/ をエスケープしない")
    }

    func testPrettyDSLNilForOtherVersion() throws {
        let data = try envelope(version: 3, dsl: #"{"name":"家計簿"}"#)
        XCTAssertNotNil(MacFeedbackContext.prettyDSL(data, version: 3), "版が同じなら出す")
        XCTAssertNil(MacFeedbackContext.prettyDSL(data, version: 4), "違う版の DSL は見せない")
        XCTAssertNil(MacFeedbackContext.prettyDSL(data, version: 0))
    }

    func testPrettyDSLNilForBrokenOrUnexpectedJSON() throws {
        XCTAssertNil(MacFeedbackContext.prettyDSL(Data("{壊れた".utf8), version: 1), "壊れた JSON は nil")
        XCTAssertNil(MacFeedbackContext.prettyDSL(Data(), version: 1), "空も nil")
        XCTAssertNil(MacFeedbackContext.prettyDSL(Data(#"[{"version":1}]"#.utf8), version: 1), "封筒の形でなければ nil")
        XCTAssertNil(MacFeedbackContext.prettyDSL(Data(#"{"appId":"mac-live","version":1}"#.utf8), version: 1), "dsl が無ければ nil")
    }

    // MARK: - osName(サーバーが先頭の名前で iOS と分ける)

    func testOSNameStartsWithMacOS() {
        XCTAssertEqual(MacFeedbackContext.osName(.init(majorVersion: 15, minorVersion: 1, patchVersion: 0)), "macOS 15.1.0")
    }

    // MARK: - scaled(長い辺を上限の画素に収める)

    private func image(width: Int, height: Int) throws -> CGImage {
        let ctx = try XCTUnwrap(CGContext(data: nil, width: width, height: height, bitsPerComponent: 8, bytesPerRow: 0,
                                          space: CGColorSpaceCreateDeviceRGB(),
                                          bitmapInfo: CGImageAlphaInfo.noneSkipLast.rawValue))
        ctx.setFillColor(NSColor.white.cgColor)
        ctx.fill(CGRect(x: 0, y: 0, width: width, height: height))
        return try XCTUnwrap(ctx.makeImage())
    }

    func testScaledFitsLongestSideInMaxPixels() throws {
        let wide = try XCTUnwrap(MacFeedbackScreenshot.scaled(try image(width: 3200, height: 1600), maxPixels: 1280))
        XCTAssertEqual(wide.width, 1280)
        XCTAssertEqual(wide.height, 640, "縦横の比は変えない")

        let tall = try XCTUnwrap(MacFeedbackScreenshot.scaled(try image(width: 800, height: 2400), maxPixels: 1280))
        XCTAssertEqual(tall.height, 1280)
        XCTAssertLessThanOrEqual(max(tall.width, tall.height), 1280)
    }

    func testScaledKeepsSmallImageAsIs() throws {
        let small = try image(width: 640, height: 480)
        let out = try XCTUnwrap(MacFeedbackScreenshot.scaled(small, maxPixels: 1280))
        XCTAssertEqual(out.width, 640)
        XCTAssertEqual(out.height, 480)
        let exact = try XCTUnwrap(MacFeedbackScreenshot.scaled(try image(width: 1280, height: 700), maxPixels: 1280))
        XCTAssertEqual(exact.width, 1280, "ちょうど上限なら縮めない")
    }

    func testScaledNeverGoesBelowOnePixel() throws {
        let out = try XCTUnwrap(MacFeedbackScreenshot.scaled(try image(width: 2000, height: 2), maxPixels: 10))
        XCTAssertEqual(out.width, 10)
        XCTAssertGreaterThanOrEqual(out.height, 1, "0 px の画像は作らない")
    }
}
