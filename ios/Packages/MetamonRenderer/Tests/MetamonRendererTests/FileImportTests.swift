import UIKit
import UniformTypeIdentifiers
import XCTest
@testable import MetamonDSL
@testable import MetamonRenderer

/// `fileImport`: 選べるファイルの種類・選ばれたファイルの見分け・PDF の本文の取り出し・登録。
final class FileImportTests: XCTestCase {
    func testContentTypesFollowAccept() {
        XCTAssertEqual(FileTextReading.contentTypes(["csv"]), [.commaSeparatedText])
        XCTAssertEqual(FileTextReading.contentTypes(["pdf", "text", "xlsx"]), [.pdf, .plainText])
        XCTAssertEqual(FileTextReading.contentTypes([]), [])
    }

    func testKindOfPickedFile() {
        let csv = URL(fileURLWithPath: "/tmp/名簿.CSV"), pdf = URL(fileURLWithPath: "/tmp/a.pdf"), txt = URL(fileURLWithPath: "/tmp/a.txt")
        XCTAssertEqual(FileTextReading.kind(of: csv, accept: ["csv"]), .csv)
        XCTAssertEqual(FileTextReading.kind(of: pdf, accept: ["csv", "pdf"]), .pdf)
        XCTAssertEqual(FileTextReading.kind(of: txt, accept: ["text"]), .text)
        XCTAssertNil(FileTextReading.kind(of: pdf, accept: ["csv"]))
        XCTAssertNil(FileTextReading.kind(of: URL(fileURLWithPath: "/tmp/a.docx"), accept: ["csv", "text", "pdf"]))
    }

    /// 文字の層がある PDF(2 ページ)の本文をページ順に取り出す。
    func testPDFTextLayer() throws {
        let data = Self.pdf(pages: ["組み立て手順 1 ネジを締める", "手順 2 Step two"])
        let text = try XCTUnwrap(FileTextReading.pdfText(data))
        XCTAssertTrue(text.contains("組み立て手順"), text)
        XCTAssertTrue(text.contains("Step two"), text)
        XCTAssertLessThan(try XCTUnwrap(text.range(of: "ネジ")).lowerBound, try XCTUnwrap(text.range(of: "Step")).lowerBound)
    }

    func testBrokenPDFIsNil() {
        XCTAssertNil(FileTextReading.pdfText(Data("not a pdf".utf8)))
    }

    func testRegisteredForTheReviewBuildToo() {
        XCTAssertTrue(ComponentRegistry.standard.registeredTypes.contains("fileImport"))
        XCTAssertFalse(ClientCapabilities.reviewExcludedComponents.contains(.fileImport))
    }

    private static func pdf(pages: [String]) -> Data {
        let renderer = UIGraphicsPDFRenderer(bounds: CGRect(x: 0, y: 0, width: 595, height: 842))
        return renderer.pdfData { ctx in
            for text in pages {
                ctx.beginPage()
                (text as NSString).draw(at: CGPoint(x: 40, y: 60), withAttributes: [.font: UIFont.systemFont(ofSize: 18)])
            }
        }
    }
}
