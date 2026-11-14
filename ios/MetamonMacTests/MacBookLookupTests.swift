import XCTest
import MetamonDSL
import MetamonRenderer
import MetamonServices
@testable import Metamon

/// Mac の本の情報(#1276): サーバーにつながっている時は appconfig の lookupBook、モックの時は手で入力の案内
@MainActor
final class MacBookLookupTests: XCTestCase {
    private struct FixedBooks: BookLookupClient {
        func lookup(isbn: String) async throws -> BookInfo {
            BookInfo(isbn: isbn, title: "坊っちゃん", authors: "夏目漱石", source: .init(name: "openBD", url: "https://openbd.jp"))
        }
    }

    private func make() throws -> (MacAppRuntime, FormDraft, Action) {
        let json = """
        {"schemaVersion":1,"app":{"name":"本棚"},"initialScreen":"form",
         "entities":[{"id":"books","name":"本","fields":[{"id":"isbn","name":"ISBN","type":"string"},
           {"id":"title","name":"書名","type":"string"},{"id":"authors","name":"著者","type":"string"}]}],
         "screens":[{"id":"form","title":"本","entity":"books","components":[{"type":"textField","field":"isbn"}]}]}
        """
        let dsl = try JSONDecoder().decode(AppDSL.self, from: Data(json.utf8))
        let rt = MacAppRuntime(appId: "test-books-\(UUID().uuidString.lowercased())", dsl: dsl,
                               ai: MockAIClient(delay: .zero), isAIConsentGranted: { false })
        let form = FormDraft(entity: try XCTUnwrap(dsl.entities.first))
        form.values["isbn"] = .string("9784003101018")
        let action = try JSONDecoder().decode(Action.self, from: Data(#"{"type":"lookupBook","bookLookup":{"isbnField":"isbn","titleField":"title","authorsField":"authors"}}"#.utf8))
        return (rt, form, action)
    }

    func testFillsFieldsFromServer() async throws {
        let (rt, form, action) = try make()
        rt.books = FixedBooks()
        await rt.run(action, ActionContext(route: Route(screen: "form"), form: form))
        XCTAssertEqual(form.values["title"], .string("坊っちゃん"))
        XCTAssertEqual(form.values["authors"], .string("夏目漱石"))
    }

    func testWithoutServerAsksToTypeByHand() async throws {
        let (rt, form, action) = try make()
        await rt.run(action, ActionContext(route: Route(screen: "form"), form: form))
        XCTAssertNotNil(rt.toast, "モックの時は手で入力の案内")
        XCTAssertEqual((form.values["title"] ?? .null).displayString, "")
    }

    func testServicesBuildBooksOnlyWhenLive() {
        let url = URL(string: "https://appconfig.example.com/")
        let live = MacServices(auth: MockAuthService(signedIn: true), ai: MockAIClient(), feedback: MockFeedbackClient(),
                               isLive: true, appConfigURL: url)
        XCTAssertTrue(live.books is HTTPBookLookupClient)
        XCTAssertNil(MacServices.make(environment: ["METAMON_MOCK": "1"]).books)
    }
}
