import XCTest
import MetamonDSL
import MetamonServices
@testable import Metamon

/// フィードバックに今のアプリの設計(DSL)を添える(#1330)。送るのは appId と版だけ、見せ方は iOS と同じ
@MainActor
final class MacFeedbackDSLTests: XCTestCase {
    private func kakeibo(version: Int) throws -> (AppDocument, Data) {
        let url = try XCTUnwrap(Bundle(for: MacAppRuntime.self).url(forResource: "kakeibo", withExtension: "json"))
        let raw = try Data(contentsOf: url)
        let envelope = try XCTUnwrap(DocumentEnvelope.make(appId: "mac-live", version: version, dsl: raw))
        return (AppDocument(appId: "mac-live", version: version, dsl: try AppDSL.load(contentsOf: url)), envelope)
    }

    func testSummaryShowsNameScreensEntitiesAndFullDSL() throws {
        let (doc, envelope) = try kakeibo(version: 4)
        let summary = try XCTUnwrap(MacFeedbackContext.appSummary(doc, envelope: envelope))
        XCTAssertEqual(summary.ref, .init(appId: "mac-live", dslVersion: 4), "送るのは appId と版だけ")
        XCTAssertEqual(summary.name, doc.dsl.app.name)
        XCTAssertEqual(summary.screens.count, doc.dsl.screens.count)
        XCTAssertEqual(summary.entities.count, doc.dsl.entities.count)
        XCTAssertTrue(summary.json?.contains("\"screens\"") == true, "送る DSL の全文を見せる")
    }

    func testFullDSLOnlyForSameVersion() throws {
        let (doc, _) = try kakeibo(version: 4)
        let (_, olderEnvelope) = try kakeibo(version: 3)
        XCTAssertNil(MacFeedbackContext.appSummary(doc, envelope: olderEnvelope)?.json, "違う版の DSL は見せない")
        XCTAssertNotNil(MacFeedbackContext.appSummary(doc, envelope: nil), "全文が無くても要約は出す")
    }

    func testNoAppWithoutServerApp() {
        let context = MacFeedbackContext.capture(screenshot: nil, app: nil)
        XCTAssertNil(context.app, "サーバーに無いアプリは添付の行を出さない")
    }

    func testCaptureUsesCurrentServerApp() throws {
        let (doc, envelope) = try kakeibo(version: 2)
        let context = MacFeedbackContext.capture(screenshot: nil, app: .init(document: doc, envelope: envelope))
        XCTAssertEqual(context.app?.ref.appId, "mac-live")
    }
}
