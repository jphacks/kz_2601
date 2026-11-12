import XCTest
import MetamonDSL
@testable import MetamonRenderer

/// 記録の文字の長押しの「翻訳」(#2000): 出す・出さない(OS の版・field の種類・秘密の field・空・審査用の版)。
final class RecordTranslationTests: XCTestCase {
    private lazy var notes: Entity = try! JSONDecoder().decode(Entity.self, from: Data("""
    {"id":"notes","name":"メモ","fields":[
      {"id":"title","name":"題名","type":"string"},
      {"id":"body","name":"本文","type":"text"},
      {"id":"secret","name":"秘密","type":"text","encrypted":true},
      {"id":"answer","name":"答え","type":"string","hidden":true},
      {"id":"pages","name":"ページ数","type":"integer"},
      {"id":"genre","name":"ジャンル","type":"enum","options":["小説","技術書"]}
    ]}
    """.utf8))

    private func f(_ id: String) -> Field { notes.field(id)! }

    func testOffersForTextFields() {
        XCTAssertTrue(RecordTranslation.offers(text: "Hello", fields: [f("title")], osSupports: true, reviewBuild: false))
        XCTAssertTrue(RecordTranslation.offers(text: "Long body", fields: [f("body")], osSupports: true, reviewBuild: false))
        // 文字の field と数値が混ざっていても出す
        XCTAssertTrue(RecordTranslation.offers(text: "Hello 12", fields: [f("title"), f("pages")], osSupports: true, reviewBuild: false))
    }

    func testHiddenBeforeIOS18() {
        XCTAssertFalse(RecordTranslation.offers(text: "Hello", fields: [f("title")], osSupports: false, reviewBuild: false))
    }

    func testHiddenForEncryptedField() {
        XCTAssertFalse(RecordTranslation.offers(text: "secret", fields: [f("secret")], osSupports: true, reviewBuild: false))
        // 秘密の field が 1 つでも混ざれば出さない
        XCTAssertFalse(RecordTranslation.offers(text: "Hello secret", fields: [f("title"), f("secret")], osSupports: true, reviewBuild: false))
    }

    func testHiddenForEmptyText() {
        XCTAssertFalse(RecordTranslation.offers(text: "", fields: [f("title")], osSupports: true, reviewBuild: false))
        XCTAssertFalse(RecordTranslation.offers(text: " \n ", fields: [f("title")], osSupports: true, reviewBuild: false))
    }

    func testHiddenForNonTextFields() {
        XCTAssertFalse(RecordTranslation.offers(text: "12", fields: [f("pages")], osSupports: true, reviewBuild: false))
        XCTAssertFalse(RecordTranslation.offers(text: "小説", fields: [f("genre")], osSupports: true, reviewBuild: false))
        // field を含まない固定の文言
        XCTAssertFalse(RecordTranslation.offers(text: "見出し", fields: [], osSupports: true, reviewBuild: false))
    }

    func testOfferedInReviewBuildToo() {
        // OS の機能だけで外への通信が無いので審査用の版でも出す
        XCTAssertTrue(RecordTranslation.offers(text: "Hello", fields: [f("title")], osSupports: true, reviewBuild: true))
        XCTAssertFalse(RecordTranslation.offers(text: "secret", fields: [f("secret")], osSupports: true, reviewBuild: true))
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
