import XCTest
import PDFKit
@testable import MetamonRenderer

/// #2227: 書類のスキャンの「ページの画像の並び → PDF」。画面(`VNDocumentCameraViewController`)は
/// 出せないので、純関数と差し替えた偽物のスキャナだけを見る。
@MainActor
final class DocumentScanTests: XCTestCase {
    /// 縦横の違うページ(順番が分かるように大きさを変える)。
    private func page(width: CGFloat, height: CGFloat) -> PlatformImage {
        PlatformGraphics.image(size: CGSize(width: width, height: height), scale: 1, opaque: true) { cg in
            cg.setFillColor(red: 1, green: 1, blue: 1, alpha: 1)
            cg.fill(CGRect(x: 0, y: 0, width: width, height: height))
            cg.setFillColor(red: 0, green: 0, blue: 0, alpha: 1)
            cg.fill(CGRect(x: 0, y: 0, width: width / 2, height: height / 2))
        }
    }

    // MARK: - 画像の並び → PDF のページ数

    func testPageCountMatchesImageCount() throws {
        for count in 1...3 {
            let images = (0..<count).map { _ in page(width: 60, height: 80) }
            let data = try XCTUnwrap(DocumentScanPDF.data(from: images), "\(count) 枚なら PDF が返る")
            let document = try XCTUnwrap(PDFDocument(data: data))
            XCTAssertEqual(document.pageCount, count, "ページ数は画像の枚数と同じ")
        }
    }

    func testTwoPageScanKeepsOrder() throws {
        // 実機の 2 ページのスキャンと同じ形。横長 → 縦長の順がそのまま 1 ページ目 → 2 ページ目になる。
        let data = try XCTUnwrap(DocumentScanPDF.data(from: [page(width: 120, height: 60), page(width: 60, height: 120)]))
        let document = try XCTUnwrap(PDFDocument(data: data))
        XCTAssertEqual(document.pageCount, 2)
        let first = try XCTUnwrap(document.page(at: 0)).bounds(for: .mediaBox)
        let second = try XCTUnwrap(document.page(at: 1)).bounds(for: .mediaBox)
        XCTAssertGreaterThan(first.width, first.height, "1 ページ目は横長のまま")
        XCTAssertGreaterThan(second.height, second.width, "2 ページ目は縦長のまま")
    }

    // MARK: - 空の時

    func testEmptyImagesMakeNoPDF() {
        XCTAssertNil(DocumentScanPDF.data(from: []), "0 ページの PDF は作らず nil(やめた時と同じ扱い)")
        XCTAssertNil(DocumentScanPDF.firstImageValue(from: []), "1 枚目も無い")
    }

    // MARK: - 大きさの上限・圧縮

    func testPageIsShrunkToPhotoLimit() throws {
        let data = try XCTUnwrap(DocumentScanPDF.data(from: [page(width: 2400, height: 1200)]))
        let document = try XCTUnwrap(PDFDocument(data: data))
        let bounds = try XCTUnwrap(document.page(at: 0)).bounds(for: .mediaBox)
        XCTAssertLessThanOrEqual(max(bounds.width, bounds.height), DocumentScanPDF.defaultMaxSide,
                                 "長辺は写真の部品の保存と同じ上限まで縮む")
        XCTAssertEqual(bounds.width / bounds.height, 2, accuracy: 0.05, "縦横の比は変えない")
    }

    func testFirstImageValueIsPhotoDataURL() throws {
        let value = try XCTUnwrap(DocumentScanPDF.firstImageValue(from: [page(width: 60, height: 80), page(width: 120, height: 60)]))
        XCTAssertTrue(value.hasPrefix("data:image/jpeg;base64,"), "写真の部品と同じ値の形")
        let image = try XCTUnwrap(PhotoCodec.image(from: .string(value)))
        XCTAssertGreaterThan(image.size.height, image.size.width, "1 枚目(縦長)が返る")
    }

    // MARK: - スキャンする口の差し替え

    func testScanReturnsPDFOfAllPages() async throws {
        let scanner = FakeDocumentScanner(pages: [page(width: 60, height: 80), page(width: 60, height: 80)])
        let scanned = await DocumentScan.pdf(using: scanner)
        let data = try XCTUnwrap(scanned)
        XCTAssertEqual(try XCTUnwrap(PDFDocument(data: data)).pageCount, 2)
        XCTAssertEqual(scanner.scanCount, 1)
    }

    func testCancelledScanReturnsNil() async {
        let scanner = FakeDocumentScanner(pages: [])
        let data = await DocumentScan.pdf(using: scanner)
        XCTAssertNil(data, "やめた時はページが 0 枚なので PDF も無い")
    }

    func testUnsupportedDeviceDoesNotPresent() async {
        let scanner = FakeDocumentScanner(isSupported: false, pages: [page(width: 60, height: 80)])
        let data = await DocumentScan.pdf(using: scanner)
        XCTAssertNil(data)
        XCTAssertEqual(scanner.scanCount, 0, "使えない端末では画面を出しに行かない")
    }
}
