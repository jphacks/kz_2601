import XCTest
import MetamonDSL
@testable import MetamonRenderer

/// #2850: photoField にファイルから入れる画像(#1035)の判定と data URL。
@MainActor
final class PhotoFileImportTests: XCTestCase {
    private var dir: URL!

    override func setUpWithError() throws {
        dir = FileManager.default.temporaryDirectory.appendingPathComponent("photo-file-import-\(UUID().uuidString)", isDirectory: true)
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
    }

    override func tearDownWithError() throws {
        try? FileManager.default.removeItem(at: dir)
    }

    private func file(_ name: String) -> URL { URL(fileURLWithPath: "/tmp/\(name)") }

    // MARK: accepts

    func testAcceptsImageExtensions() {
        for name in ["a.png", "a.jpg", "a.jpeg", "a.heic", "a.gif", "a.tiff"] {
            XCTAssertTrue(PhotoFileImport.accepts(file(name)), name)
        }
        XCTAssertTrue(PhotoFileImport.accepts(file("A.PNG")), "大文字の拡張子")
        XCTAssertTrue(PhotoFileImport.accepts(file("写真 1.JPG")))
    }

    func testRejectsNonImages() {
        XCTAssertFalse(PhotoFileImport.accepts(file("memo.txt")))
        XCTAssertFalse(PhotoFileImport.accepts(file("doc.pdf")))
        XCTAssertFalse(PhotoFileImport.accepts(file("clip.mov")))
        XCTAssertFalse(PhotoFileImport.accepts(file("noext")), "拡張子なし")
        XCTAssertFalse(PhotoFileImport.accepts(file("weird.zzzunknown")), "知らない拡張子")
    }

    func testRejectsNonFileURL() throws {
        XCTAssertFalse(PhotoFileImport.accepts(try XCTUnwrap(URL(string: "https://example.com/a.png"))), "Web の URL は受け付けない")
    }

    // MARK: dataURL(fileAt:)

    func testDataURLFromSmallImageFile() throws {
        let img = testImage(width: 40, height: 20) { ctx in
            PlatformColor.systemTeal.setFill(); ctx.fill(CGRect(x: 0, y: 0, width: 40, height: 20))
        }
        let url = dir.appendingPathComponent("small.png")
        try XCTUnwrap(img.pngData()).write(to: url)

        let encoded = try XCTUnwrap(PhotoFileImport.dataURL(fileAt: url))
        XCTAssertTrue(encoded.hasPrefix("data:image/jpeg;base64,"), "撮影と同じ JPEG の data URL")
        let decoded = try XCTUnwrap(PhotoCodec.image(from: .string(encoded)))
        XCTAssertEqual(decoded.size.width * decoded.scale, 40, accuracy: 1, "小さい画像は拡大しない")
        XCTAssertTrue(FileManager.default.fileExists(atPath: url.path), "元のファイルは消さない")
    }

    func testDataURLShrinksLargeImage() throws {
        let img = testImage(width: 1600, height: 400) { ctx in
            PlatformColor.systemOrange.setFill(); ctx.fill(CGRect(x: 0, y: 0, width: 1600, height: 400))
        }
        let url = dir.appendingPathComponent("wide.png")
        try XCTUnwrap(img.pngData()).write(to: url)
        let decoded = try XCTUnwrap(PhotoCodec.image(from: .string(try XCTUnwrap(PhotoFileImport.dataURL(fileAt: url)))))
        XCTAssertLessThanOrEqual(decoded.size.width * decoded.scale, 801, "長い辺は 800px まで")
    }

    func testDataURLMissingFileIsNil() {
        XCTAssertNil(PhotoFileImport.dataURL(fileAt: dir.appendingPathComponent("missing.png")))
    }

    func testDataURLNonImageIsNil() throws {
        let txt = dir.appendingPathComponent("memo.txt")
        try Data("hello".utf8).write(to: txt)
        XCTAssertNil(PhotoFileImport.dataURL(fileAt: txt), "画像でない拡張子は読まない")

        let fake = dir.appendingPathComponent("fake.png")
        try Data("not an image".utf8).write(to: fake)
        XCTAssertNil(PhotoFileImport.dataURL(fileAt: fake), "拡張子だけ画像で中身が違えば nil")
    }
}
