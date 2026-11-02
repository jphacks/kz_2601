#if !APP_STORE_REVIEW
import XCTest
import UIKit
import MetamonDSL
import MetamonServices
@testable import Metamon

/// #319 フィードバック: 送れる DSL は構造だけ(記録は含まない)・スクショは上限以下の JPEG・初期状態は DSL を出さない。
@MainActor
final class FeedbackTests: XCTestCase {
    private let json = """
    {"schemaVersion":1,"app":{"name":"日記","voiceEntry":"floating"},"initialScreen":"home",
     "entities":[{"id":"diary","name":"日記","fields":[{"id":"body","name":"本文","type":"text"}]}],
     "screens":[{"id":"home","title":"一覧","components":[{"type":"list","entity":"diary"}]}]}
    """

    private func doc(version: Int = 3) throws -> AppDocument {
        AppDocument(appId: "fb-test", version: version, dsl: try JSONDecoder().decode(AppDSL.self, from: Data(json.utf8)))
    }

    func testAppSummaryIsStructureOnly() throws {
        let dir = FileManager.default.temporaryDirectory.appendingPathComponent("fb-\(UUID().uuidString)")
        let cache = DocumentCache(directory: dir)
        defer { try? FileManager.default.removeItem(at: dir) }
        cache.store(appId: "fb-test", envelope: Data(#"{"appId":"fb-test","version":3,"dsl":\#(json)}"#.utf8))

        let s = try XCTUnwrap(FeedbackContext.appSummary(try doc(), cache: cache))
        XCTAssertEqual(s.ref, .init(appId: "fb-test", dslVersion: 3))
        XCTAssertEqual(s.name, "日記")
        XCTAssertEqual(s.screens, ["一覧"])
        XCTAssertEqual(s.entities, ["日記(本文)"])
        XCTAssertTrue(s.json?.contains("\"initialScreen\"") == true)
        XCTAssertFalse(s.json?.contains("\"appId\"") == true, "封筒ではなく dsl だけを見せる")

        // 版が違うキャッシュは見せない(サーバーが写すのは ref の版)
        XCTAssertNil(FeedbackContext.appSummary(try doc(version: 4), cache: cache)?.json)
    }

    func testVoiceOnlyAppHasNoDSLAttachment() {
        XCTAssertNil(FeedbackContext.appSummary(.fallback))
    }

    func testScreenshotIsShrunkUnderLimit() throws {
        let size = CGSize(width: 1200, height: 2600)
        let format = UIGraphicsImageRendererFormat()
        format.scale = 1
        let noisy = UIGraphicsImageRenderer(size: size, format: format).image { ctx in
            for y in stride(from: 0, to: Int(size.height), by: 6) {
                for x in stride(from: 0, to: Int(size.width), by: 6) {
                    UIColor(hue: CGFloat((x * 7 + y * 13) % 360) / 360, saturation: 1, brightness: CGFloat((x ^ y) % 100) / 100, alpha: 1).setFill()
                    ctx.fill(CGRect(x: x, y: y, width: 6, height: 6))
                }
            }
        }
        let limit = 120 << 10
        let data = try XCTUnwrap(FeedbackScreenshot.jpeg(noisy, maxBytes: limit))
        XCTAssertLessThanOrEqual(data.count, limit)
        XCTAssertEqual(Array(data.prefix(3)), [0xFF, 0xD8, 0xFF], "JPEG で送る")
    }

    func testDeviceInfoIsFilled() {
        let d = FeedbackContext.currentDevice()
        XCTAssertFalse(d.model.isEmpty)
        XCTAssertTrue(d.os.contains("iOS") || d.os.contains("iPadOS"))
    }

    func testRateLimitMessage() {
        let err = APIError.server(status: 429, code: "rate_limited", message: "m", details: ["scope": .string("user")])
        XCTAssertTrue(FeedbackComposerView.message(for: err).contains("明日") || FeedbackComposerView.message(for: err).contains("tomorrow"))
    }
}
#endif
