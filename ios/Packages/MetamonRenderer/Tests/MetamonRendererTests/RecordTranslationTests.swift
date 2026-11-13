import XCTest
import MetamonDSL
@testable import MetamonRenderer

/// #2849: 記録の長押しの「翻訳」(#2000)を出すかの判定。
final class RecordTranslationTests: XCTestCase {
    private func entity(_ titleField: String? = nil) throws -> Entity {
        let tf = titleField.map { #""titleField":"\#($0)","# } ?? ""
        return try JSONDecoder().decode(Entity.self, from: Data(#"""
        {"id":"note","name":"メモ",\#(tf)"fields":[
          {"id":"secret","name":"隠し","type":"string","hidden":true},
          {"id":"title","name":"題","type":"string"},
          {"id":"body","name":"本文","type":"text"},
          {"id":"pin","name":"暗証","type":"string","encrypted":true},
          {"id":"n","name":"数","type":"integer"},
          {"id":"twice","name":"2倍","type":"integer","computed":{"op":"mul","args":[{"ref":"n"},2]}},
          {"id":"label","name":"見出し","type":"string","computed":{"op":"concat","args":[{"ref":"title"},"!"]}},
          {"id":"pic","name":"写真","type":"photo"},
          {"id":"day","name":"日","type":"date"}]}
        """#.utf8))
    }

    private func fields(_ ids: [String]) throws -> [Field] {
        let e = try entity()
        return try ids.map { try XCTUnwrap(e.field($0), $0) }
    }

    // MARK: offers

    func testOffersForTextFields() throws {
        XCTAssertTrue(RecordTranslation.offers(text: "Hello", fields: try fields(["title"]), osSupports: true))
        XCTAssertTrue(RecordTranslation.offers(text: "Hello", fields: try fields(["body"]), osSupports: true))
        XCTAssertTrue(RecordTranslation.offers(text: "3 個の Hello", fields: try fields(["n", "title"]), osSupports: true),
                      "文字の field が 1 つでもあれば出す")
    }

    func testDoesNotOfferForNonTextFields() throws {
        XCTAssertFalse(RecordTranslation.offers(text: "42", fields: try fields(["n"]), osSupports: true), "数だけ")
        XCTAssertFalse(RecordTranslation.offers(text: "2026-11-13", fields: try fields(["day"]), osSupports: true))
        XCTAssertFalse(RecordTranslation.offers(text: "x", fields: [], osSupports: true), "field が無い")
    }

    func testDoesNotOfferWhenOSUnsupported() throws {
        XCTAssertFalse(RecordTranslation.offers(text: "Hello", fields: try fields(["title"]), osSupports: false))
    }

    func testDoesNotOfferForEmptyText() throws {
        XCTAssertFalse(RecordTranslation.offers(text: "", fields: try fields(["title"]), osSupports: true))
        XCTAssertFalse(RecordTranslation.offers(text: " \n\t", fields: try fields(["title"]), osSupports: true), "空白だけ")
    }

    func testDoesNotOfferForEncryptedOrComputed() throws {
        XCTAssertFalse(RecordTranslation.offers(text: "Hello 1234", fields: try fields(["title", "pin"]), osSupports: true),
                       "秘密の field を含む")
        XCTAssertFalse(RecordTranslation.offers(text: "Hello!", fields: try fields(["label"]), osSupports: true),
                       "計算の field(文字)")
        XCTAssertFalse(RecordTranslation.offers(text: "Hello 6", fields: try fields(["title", "twice"]), osSupports: true),
                       "計算の field を含む")
    }

    func testIsText() throws {
        XCTAssertEqual(try fields(["title", "body", "pin", "n", "pic", "day"]).map(RecordTranslation.isText),
                       [true, true, true, false, false, false])
    }

    // MARK: substitutedFields

    func testSubstitutedFieldsFromTemplate() throws {
        let e = try entity()
        XCTAssertEqual(RecordTranslation.substitutedFields(template: "{{title}}: {{body}} ({{n}})", entity: e).map(\.id),
                       ["title", "body", "n"], "entity の field の順")
        XCTAssertEqual(RecordTranslation.substitutedFields(template: "{{body}} / {{title}}", entity: e).map(\.id), ["title", "body"])
    }

    func testSubstitutedFieldsSkipsHiddenUnsupportedAndUnknown() throws {
        let e = try entity()
        XCTAssertEqual(RecordTranslation.substitutedFields(template: "{{secret}} {{pic}} {{nope}} {title}", entity: e).map(\.id), [],
                       "隠す field・差し込まない種類・無い field・波かっこ 1 つは対象外")
        XCTAssertEqual(RecordTranslation.substitutedFields(template: "固定の文", entity: e).map(\.id), [])
    }

    // MARK: rowFields

    private func item(_ json: String) throws -> ListItem {
        try JSONDecoder().decode(ListItem.self, from: Data(json.utf8))
    }

    func testRowFieldsTitleAndSubtitle() throws {
        let e = try entity()
        let fs = RecordTranslation.rowFields(item: try item(#"{"title":"title","subtitle":"body","trailing":"n"}"#), entity: e)
        XCTAssertEqual(fs.map(\.id), ["title", "body"], "trailing は訳さない")
    }

    func testRowFieldsDropsUnknownField() throws {
        let fs = RecordTranslation.rowFields(item: try item(#"{"title":"nope","subtitle":"body"}"#), entity: try entity())
        XCTAssertEqual(fs.map(\.id), ["body"])
    }

    func testRowFieldsWithoutItemUsesDisplayTitle() throws {
        XCTAssertEqual(RecordTranslation.rowFields(item: nil, entity: try entity()).map(\.id), ["title"],
                       "行の指定が無ければ見出しの field(隠す field は除く)")
        XCTAssertEqual(RecordTranslation.rowFields(item: nil, entity: try entity("body")).map(\.id), ["body"])
    }

    func testRowFieldsWithoutEntityIsEmpty() throws {
        XCTAssertEqual(RecordTranslation.rowFields(item: try item(#"{"title":"title"}"#), entity: nil).map(\.id), [])
        XCTAssertEqual(RecordTranslation.rowFields(item: nil, entity: nil).map(\.id), [])
    }
}
