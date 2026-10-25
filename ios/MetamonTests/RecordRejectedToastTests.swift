import XCTest
import MetamonDSL
import MetamonServices
@testable import Metamon

/// 同期で保存が弾かれた時(422 invalid_record)の toast。どの項目かを DSL の名前で出す。
@MainActor
final class RecordRejectedToastTests: XCTestCase {
    private func runtime() throws -> AppRuntime {
        let doc = try DSLExamples.document("warikan", version: 1)
        return AppRuntime(document: doc, services: BuildConfig.make(environment: .mock))
    }

    func testRejectedToastNamesTheField() throws {
        let rt = try runtime()
        let text = rt.rejectedToast(entity: "expenses", field: "payer", message: "field \"payer\" is required")
        XCTAssertTrue(text.contains("払った人"), text)
        XCTAssertFalse(text.contains("\"payer\""), "英語の field id は見せない: \(text)")
        XCTAssertTrue(text.contains("is required"), "理由は残す: \(text)")
    }

    func testRejectedToastWithoutFieldKeepsMessage() throws {
        let rt = try runtime()
        let text = rt.rejectedToast(entity: "expenses", field: nil, message: "data is required")
        XCTAssertTrue(text.contains("data is required"), text)
    }

    func testRejectedToastUnknownFieldFallsBackToId() throws {
        let rt = try runtime()
        let text = rt.rejectedToast(entity: "expenses", field: "gone", message: "field \"gone\" must be a string")
        XCTAssertTrue(text.contains("gone"), text)
    }
}
