import XCTest
import MetamonDSL
import MetamonRenderer
import MetamonServices
@testable import Metamon

/// lookupBook: 結果を待つ間にフォームが変わったら、古い結果で上書きしない(PR #720 のレビュー)。
@MainActor
final class BookLookupFlowTests: XCTestCase {
    /// `release()` まで応答を止める偽の検索。
    private actor HeldBooks: BookLookupClient {
        private(set) var called = false
        private var released = false
        func lookup(isbn: String) async throws -> BookInfo {
            called = true
            while !released { await Task.yield() }
            return BookInfo(isbn: isbn, title: "坊っちゃん", authors: "夏目漱石", source: .init(name: "openBD", url: "https://openbd.jp"))
        }
        func release() { released = true }
    }

    private let isbn = "9784003101018"

    private func make() throws -> (AppRuntime, HeldBooks, FormDraft, Action) {
        let json = """
        {"schemaVersion":1,"app":{"name":"本棚"},"initialScreen":"form",
         "entities":[{"id":"books","name":"本","fields":[{"id":"isbn","name":"ISBN","type":"string"},
           {"id":"title","name":"書名","type":"string"},{"id":"authors","name":"著者","type":"string"}]}],
         "screens":[{"id":"form","title":"本","entity":"books","components":[{"type":"textField","field":"isbn"}]}]}
        """
        let dsl = try JSONDecoder().decode(AppDSL.self, from: Data(json.utf8))
        var services = BuildConfig.make(environment: .mock)
        let books = HeldBooks()
        services.books = books
        let rt = AppRuntime(document: AppDocument(appId: "test-books-\(UUID().uuidString.lowercased())", version: 1, dsl: dsl), services: services)
        let form = FormDraft(entity: try XCTUnwrap(dsl.entities.first))
        form.values["isbn"] = .string(isbn)
        let action = try JSONDecoder().decode(Action.self, from: Data(#"{"type":"lookupBook","bookLookup":{"isbnField":"isbn","titleField":"title","authorsField":"authors"}}"#.utf8))
        return (rt, books, form, action)
    }

    private func run(_ rt: AppRuntime, _ books: HeldBooks, _ form: FormDraft, _ action: Action, meanwhile: () -> Void) async {
        let task = Task { await rt.lookupBook(action, ActionContext(route: Route(screen: "form"), form: form)) }
        while !(await books.called) { await Task.yield() }
        meanwhile()
        await books.release()
        await task.value
    }

    func testFillsFieldsWhenUnchanged() async throws {
        let (rt, books, form, action) = try make()
        await run(rt, books, form, action) {}
        XCTAssertEqual(form.values["title"], .string("坊っちゃん"))
        XCTAssertEqual(form.values["authors"], .string("夏目漱石"))
    }

    func testKeepsFieldTypedWhileWaiting() async throws {
        let (rt, books, form, action) = try make()
        await run(rt, books, form, action) { form.values["title"] = .string("手で入れた書名") }
        XCTAssertEqual(form.values["title"], .string("手で入れた書名"), "待つ間に打った欄は上書きしない")
        XCTAssertEqual(form.values["authors"], .string("夏目漱石"))
    }

    func testIgnoresResultAfterISBNChanged() async throws {
        let (rt, books, form, action) = try make()
        await run(rt, books, form, action) { form.values["isbn"] = .string("9784101010014") }
        XCTAssertEqual(form.values["isbn"], .string("9784101010014"), "打ち直した ISBN を戻さない")
        XCTAssertEqual((form.values["title"] ?? .null).displayString, "")
    }

    func testIgnoresResultAfterMovingToAnotherRecord() async throws {
        let (rt, books, form, action) = try make()
        await run(rt, books, form, action) {
            form.load(Record(id: "other", data: ["isbn": .string(isbn), "title": .string("別の本")]))
        }
        XCTAssertEqual(form.values["title"], .string("別の本"))
        XCTAssertEqual((form.values["authors"] ?? .null).displayString, "")
    }

    func testIgnoresResultWhenBecameReadOnly() async throws {
        let (rt, books, form, action) = try make()
        await run(rt, books, form, action) { form.isReadOnly = true }
        XCTAssertEqual((form.values["title"] ?? .null).displayString, "")
    }
}
