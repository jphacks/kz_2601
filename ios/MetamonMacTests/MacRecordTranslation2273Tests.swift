import XCTest
import MetamonDSL
@testable import MetamonRenderer

/// #2273: 記録の詳細の文字の右クリックの「翻訳」(macOS 15+ だけ・14 では出さない)。
final class MacRecordTranslation2273Tests: XCTestCase {
    private lazy var notes: Entity = try! JSONDecoder().decode(Entity.self, from: Data("""
    {"id":"notes","name":"メモ","fields":[
      {"id":"title","name":"題名","type":"string"},
      {"id":"secret","name":"秘密","type":"text","encrypted":true}
    ]}
    """.utf8))

    private func f(_ id: String) -> Field { notes.field(id)! }

    func testMacThresholdIsFifteen() {
        XCTAssertFalse(RecordTranslation.osSupports(isMac: true, majorVersion: 14))
        XCTAssertTrue(RecordTranslation.osSupports(isMac: true, majorVersion: 15))
        XCTAssertTrue(RecordTranslation.osSupports(isMac: true, majorVersion: 26))
        // iOS は今までどおり 18 から
        XCTAssertFalse(RecordTranslation.osSupports(isMac: false, majorVersion: 17))
        XCTAssertTrue(RecordTranslation.osSupports(isMac: false, majorVersion: 18))
    }

    func testRuntimeMatchesThisMac() {
        let major = ProcessInfo.processInfo.operatingSystemVersion.majorVersion
        XCTAssertEqual(RecordTranslation.osSupports, RecordTranslation.osSupports(isMac: true, majorVersion: major))
        if #available(macOS 15, *) {
            XCTAssertTrue(RecordTranslation.osSupports)
        } else {
            XCTAssertFalse(RecordTranslation.osSupports)
        }
    }

    /// 出す / 出さないの決まりは iOS と同じ(Mac 14 = osSupports false では文字の field でも出さない)
    func testOffersOnlyForPlainTextFieldsWhenSupported() {
        XCTAssertTrue(RecordTranslation.offers(text: "Hello", fields: [f("title")], osSupports: true))
        XCTAssertFalse(RecordTranslation.offers(text: "Hello", fields: [f("title")], osSupports: false))
        XCTAssertFalse(RecordTranslation.offers(text: "secret", fields: [f("secret")], osSupports: true))
        XCTAssertFalse(RecordTranslation.offers(text: "  ", fields: [f("title")], osSupports: true))
    }
}
