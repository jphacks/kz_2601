import XCTest
import UniformTypeIdentifiers
@testable import MetamonRenderer
#if os(macOS)
import AppKit
#endif

/// #1333: 画像の「コピー・画像を保存…」の保存するデータ・ファイル名・クリップボード。
@MainActor
final class ImageExportTests: XCTestCase {
    private func sample() -> PlatformImage {
        PlatformGraphics.image(size: CGSize(width: 4, height: 3), scale: 1, opaque: true) { cg in
            cg.setFillColor(red: 1, green: 0, blue: 0, alpha: 1)
            cg.fill(CGRect(x: 0, y: 0, width: 4, height: 3))
        }
    }

    // 2026-11-11 01:02:03 UTC
    private let date = Date(timeIntervalSince1970: 1_794_358_923)
    private let utc = TimeZone(identifier: "UTC")!

    func testFileNameIsTitleAndTimestamp() {
        XCTAssertEqual(ImageExport.fileName(title: "レシート", fallback: "写真", date: date, timeZone: utc),
                       "レシート 2026-11-11 01.02.03")
    }

    func testFileNameFallsBackWhenTitleEmpty() {
        XCTAssertEqual(ImageExport.fileName(title: nil, fallback: "写真", date: date, timeZone: utc), "写真 2026-11-11 01.02.03")
        XCTAssertEqual(ImageExport.fileName(title: "  ", fallback: "Photo", date: date, timeZone: utc), "Photo 2026-11-11 01.02.03")
    }

    func testFileNameReplacesUnsafeCharacters() {
        XCTAssertEqual(ImageExport.fileName(title: ".a/b:c\nd", fallback: "写真", date: date, timeZone: utc),
                       "a-b-c d 2026-11-11 01.02.03", "/ と : は -、改行は空白、先頭の . は除く")
    }

    func testFileNameTruncatesLongTitle() {
        let name = ImageExport.fileName(title: String(repeating: "あ", count: 100), fallback: "写真", date: date, timeZone: utc)
        XCTAssertEqual(name, String(repeating: "あ", count: 60) + " 2026-11-11 01.02.03")
    }

    func testDataIsPNGOrJPEG() throws {
        let image = sample()
        let png = try XCTUnwrap(ImageExport.data(from: image, as: .png))
        XCTAssertEqual(Array(png.prefix(4)), [0x89, 0x50, 0x4E, 0x47])
        let jpeg = try XCTUnwrap(ImageExport.data(from: image, as: .jpeg))
        XCTAssertEqual(Array(jpeg.prefix(2)), [0xFF, 0xD8])
        XCTAssertNil(ImageExport.data(from: image, as: .gif), "JPEG / PNG 以外は作らない")
        XCTAssertEqual(ImageExport.contentTypes.first, .png, "既定は PNG(劣化しない)")
    }

    func testDocumentHoldsBothFormats() throws {
        let doc = ImageFileDocument(image: sample())
        XCTAssertEqual(Array(try XCTUnwrap(doc.png).prefix(4)), [0x89, 0x50, 0x4E, 0x47])
        XCTAssertEqual(Array(try XCTUnwrap(doc.jpeg).prefix(2)), [0xFF, 0xD8])
    }

    #if os(macOS)
    func testPasteboardImageRoundTripReplacesPreviousContents() throws {
        let board = NSPasteboard(name: NSPasteboard.Name("metamon.test.\(UUID().uuidString)"))
        defer { board.releaseGlobally() }
        PlatformPasteboard.setString("前の文字", on: board)
        PlatformPasteboard.setImage(sample(), on: board)
        XCTAssertNil(PlatformPasteboard.string(on: board), "前の中身は置き換える")
        let types = board.types ?? []
        XCTAssertTrue(types.contains(.png) && types.contains(.tiff), "PNG と TIFF の両方を載せる")
        let read = try XCTUnwrap(PlatformPasteboard.image(on: board))
        XCTAssertEqual(read.cgImage?.width, 4)
        XCTAssertEqual(read.cgImage?.height, 3)
    }
    #endif
}
