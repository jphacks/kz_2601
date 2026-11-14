import XCTest
import MetamonDSL
import MetamonServices
@testable import Metamon

/// 同期で保存が弾かれた時(422 invalid_record)の toast。どの項目かを DSL の名前で出す。
/// サーバーの英語の理由は画面に出さない(H9・ログにだけ残す)。
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
        XCTAssertFalse(text.contains("is required"), "サーバーの文は見せない: \(text)")
    }

    func testRejectedToastWithoutFieldHidesServerMessage() throws {
        let rt = try runtime()
        let text = rt.rejectedToast(entity: "expenses", field: nil, message: "data is required")
        XCTAssertFalse(text.contains("data is required"), text)
        XCTAssertEqual(text, L10n.tr("保存できませんでした。入力を確認して、もう一度お試しください"))
    }

    func testRejectedToastUnknownFieldFallsBackToId() throws {
        let rt = try runtime()
        let text = rt.rejectedToast(entity: "expenses", field: "gone", message: "field \"gone\" must be a string")
        XCTAssertTrue(text.contains("gone"), text)
    }
}
