import XCTest
import MetamonDSL
@testable import MetamonRenderer

/// 記録の文字の長押しの「翻訳」(#2000): 出す・出さない(OS の版・field の種類・秘密の field・計算の field・空)。
final class RecordTranslationTests: XCTestCase {
    private lazy var notes: Entity = try! JSONDecoder().decode(Entity.self, from: Data("""
    {"id":"notes","name":"メモ","fields":[
      {"id":"title","name":"題名","type":"string"},
      {"id":"body","name":"本文","type":"text"},
      {"id":"secret","name":"秘密","type":"text","encrypted":true},
      {"id":"answer","name":"答え","type":"string","hidden":true},
      {"id":"pages","name":"ページ数","type":"integer"},
      {"id":"genre","name":"ジャンル","type":"enum","options":["小説","技術書"]},
      {"id":"shout","name":"叫び","type":"string","computed":{"op":"concat","args":[{"ref":"secret"},"!"]}},
      {"id":"loud","name":"大声","type":"string","computed":{"op":"concat","args":[{"ref":"title"},"!"]}}
    ]}
    """.utf8))

    private func f(_ id: String) -> Field { notes.field(id)! }

    func testOffersForTextFields() {
        XCTAssertTrue(RecordTranslation.offers(text: "Hello", fields: [f("title")], osSupports: true))
        XCTAssertTrue(RecordTranslation.offers(text: "Long body", fields: [f("body")], osSupports: true))
        // 文字の field と数値が混ざっていても出す
        XCTAssertTrue(RecordTranslation.offers(text: "Hello 12", fields: [f("title"), f("pages")], osSupports: true))
    }

    func testHiddenBeforeIOS18() {
        XCTAssertFalse(RecordTranslation.offers(text: "Hello", fields: [f("title")], osSupports: false))
    }

    func testHiddenForEncryptedField() {
        XCTAssertFalse(RecordTranslation.offers(text: "secret", fields: [f("secret")], osSupports: true))
        // 秘密の field が 1 つでも混ざれば出さない
        XCTAssertFalse(RecordTranslation.offers(text: "Hello secret", fields: [f("title"), f("secret")], osSupports: true))
    }

    func testHiddenForEmptyText() {
        XCTAssertFalse(RecordTranslation.offers(text: "", fields: [f("title")], osSupports: true))
        XCTAssertFalse(RecordTranslation.offers(text: " \n ", fields: [f("title")], osSupports: true))
    }

    func testHiddenForNonTextFields() {
        XCTAssertFalse(RecordTranslation.offers(text: "12", fields: [f("pages")], osSupports: true))
        XCTAssertFalse(RecordTranslation.offers(text: "小説", fields: [f("genre")], osSupports: true))
        // field を含まない固定の文言
        XCTAssertFalse(RecordTranslation.offers(text: "見出し", fields: [], osSupports: true))
    }

    func testHiddenForComputedField() {
        // 秘密の field を参照する計算の field
        XCTAssertFalse(RecordTranslation.offers(text: "secret!", fields: [f("shout")], osSupports: true))
        // 秘密の field を参照しない計算の field も出さない(依存は辿らない)
        XCTAssertFalse(RecordTranslation.offers(text: "Hello!", fields: [f("loud")], osSupports: true))
        // 文字の field と混ざっていても出さない
        XCTAssertFalse(RecordTranslation.offers(text: "Hello Hello!", fields: [f("title"), f("loud")], osSupports: true))
    }

    func testRowFieldsWithComputedSubtitleAreNotOffered() {
        let item = try! JSONDecoder().decode(ListItem.self, from: Data(#"{"title":"title","subtitle":"shout"}"#.utf8))
        let fields = RecordTranslation.rowFields(item: item, entity: notes)
        XCTAssertEqual(fields.map(\.id), ["title", "shout"])
        XCTAssertFalse(RecordTranslation.offers(text: "Hello\nsecret!", fields: fields, osSupports: true))
    }

    func testSubstitutedFieldsSkipHiddenAndUnreferenced() {
        let ids = RecordTranslation.substitutedFields(template: "{{title}} / {{answer}} / {{secret}}", entity: notes).map(\.id)
        XCTAssertEqual(ids, ["title", "secret"])
    }

    func testRowFieldsUseTitleAndSubtitle() {
        let item = try! JSONDecoder().decode(ListItem.self, from: Data(#"{"title":"title","subtitle":"secret","trailing":"pages"}"#.utf8))
        XCTAssertEqual(RecordTranslation.rowFields(item: item, entity: notes).map(\.id), ["title", "secret"])
        XCTAssertEqual(RecordTranslation.rowFields(item: nil, entity: nil).map(\.id), [])
    }

    func testOSSupportMatchesRuntime() {
        if #available(iOS 18, *) {
            XCTAssertTrue(RecordTranslation.osSupports)
        } else {
            XCTAssertFalse(RecordTranslation.osSupports)
        }
    }
}
