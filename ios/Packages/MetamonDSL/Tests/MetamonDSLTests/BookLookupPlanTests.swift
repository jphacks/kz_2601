import Foundation
import XCTest
@testable import MetamonDSL

/// lookupBook: ISBN の確かめと、見つかった本 → フォームの値。
final class BookLookupPlanTests: XCTestCase {
    private func entity() throws -> Entity {
        try JSONDecoder().decode(Entity.self, from: Data(#"""
        {"id":"book","name":"本","titleField":"title","fields":[
          {"id":"isbn","name":"ISBN","type":"string"},
          {"id":"title","name":"書名","type":"string"},
          {"id":"author","name":"著者","type":"text"},
          {"id":"publisher","name":"出版社","type":"string"},
          {"id":"published","name":"出版日","type":"date"},
          {"id":"pages","name":"ページ","type":"integer"}]}
        """#.utf8))
    }

    private let book = BookInfo(isbn: "9784780802047", title: "おにぎりレシピ101", authors: "山田玲子／著 水野菜生／英訳",
                                publisher: "ポット出版", publishedDate: "2014-04", source: .init(name: "openBD", url: "https://openbd.jp/"))

    func testNormalizeISBN() {
        XCTAssertEqual(BookLookupPlan.normalizeISBN("978-4-7808-0204-7"), "9784780802047")
        XCTAssertEqual(BookLookupPlan.normalizeISBN("4780802040"), "9784780802047", "ISBN-10 は 13 桁に")
        XCTAssertEqual(BookLookupPlan.normalizeISBN("4-7808-0204-0"), "9784780802047")
        XCTAssertEqual(BookLookupPlan.normalizeISBN("080442957X"), "9780804429573", "末尾の X")
        for bad in ["", "9784780802048", "4780802041", "4900000000004", "X780802040", "978478080204", "abc", "https://evil.example/"] {
            XCTAssertNil(BookLookupPlan.normalizeISBN(bad), bad)
        }
    }

    func testValuesFillMappedFields() throws {
        let spec = BookLookupSpec(isbnField: "isbn", titleField: "title", authorsField: "author",
                                  publisherField: "publisher", publishedDateField: "published")
        let v = BookLookupPlan.values(spec, book: book, entity: try entity())
        XCTAssertEqual(v["title"], .string("おにぎりレシピ101"))
        XCTAssertEqual(v["author"], .string("山田玲子／著 水野菜生／英訳"))
        XCTAssertEqual(v["publisher"], .string("ポット出版"))
        XCTAssertEqual(v["published"], .string("2014-04-01"), "date の field は月だけでも日付に")
        XCTAssertNil(v["isbn"])
    }

    func testValuesSkipMissingWrongTypeAndEmpty() throws {
        var b = book
        b.publisher = "  "
        let spec = BookLookupSpec(isbnField: "isbn", titleField: "pages", authorsField: "nope", publisherField: "publisher")
        XCTAssertEqual(BookLookupPlan.values(spec, book: b, entity: try entity()), [:])
    }

    func testDateString() {
        XCTAssertEqual(BookLookupPlan.dateString("2014"), "2014-01-01")
        XCTAssertEqual(BookLookupPlan.dateString("2014-04-08"), "2014-04-08")
        XCTAssertNil(BookLookupPlan.dateString("2014-13"))
        XCTAssertNil(BookLookupPlan.dateString("平成26年"))
    }

    func testDecodeAction() throws {
        let raw = #"{"type":"lookupBook","bookLookup":{"isbnField":"isbn","titleField":"title"}}"#
        let a = try JSONDecoder().decode(Action.self, from: Data(raw.utf8))
        XCTAssertEqual(a.type, .lookupBook)
        XCTAssertEqual(a.bookLookup, BookLookupSpec(isbnField: "isbn", titleField: "title"))
    }

    func testDecodeResponse() throws {
        let raw = #"{"isbn":"9784780802047","title":"おにぎりレシピ101","publisher":"ポット出版","publishedDate":"2014-04-08","source":{"name":"openBD","url":"https://openbd.jp/","license":"l"},"fetchedAt":"2026-11-08T03:00:00Z"}"#
        let b = try JSONDecoder().decode(BookInfo.self, from: Data(raw.utf8))
        XCTAssertEqual(b.title, "おにぎりレシピ101")
        XCTAssertNil(b.authors)
        XCTAssertEqual(b.source.license, "l")
    }
}
