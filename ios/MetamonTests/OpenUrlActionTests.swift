import XCTest
import MetamonDSL
import MetamonServices
@testable import MetamonRenderer
@testable import Metamon

/// `openUrl` アクションも openUrlButton と同じ `ExternalURLPolicy` を通す。外に渡す処理は差し替えて確かめる。
@MainActor
final class OpenUrlActionTests: XCTestCase {
    private func runtime() throws -> (AppRuntime, () -> [URL]) {
        let doc = try DSLExamples.document("warikan", version: 1)
        let rt = AppRuntime(document: doc, services: BuildConfig.make(environment: .mock))
        var opened: [URL] = []
        rt.openExternalURL = { opened.append($0); return true }
        return (rt, { opened })
    }

    private func openUrl(_ url: String?) -> Action {
        var action = Action(type: .openUrl)
        action.url = url
        return action
    }

    private var ctx: ActionContext { ActionContext(route: Route(screen: "home", stack: Route.tabStack("home"))) }

    func testAllowedURLsAreOpened() async throws {
        for s in ["https://example.com/a?b=1", "line://nv/chat", "tel:0761234567", "mailto:a@b.c", "HTTPS://EXAMPLE.COM"] {
            let (rt, opened) = try runtime()
            await rt.run(openUrl(s), ctx)
            XCTAssertEqual(opened().map(\.absoluteString), [s], s)
            XCTAssertNil(rt.toast, s)
        }
    }

    func testDisallowedURLsAreNotOpened() async throws {
        for s in ["http://example.com", "ftp://example.com", "javascript:alert(1)", "file:///tmp/x", "telegram://x", "tailorit://import/x", "", "not a url", "%%%"] {
            let (rt, opened) = try runtime()
            await rt.run(openUrl(s), ctx)
            XCTAssertTrue(opened().isEmpty, s)
            XCTAssertNotNil(rt.toast, "拒んだことを知らせる: \(s)")
        }
    }

    func testMissingURLIsNotOpened() async throws {
        let (rt, opened) = try runtime()
        await rt.run(openUrl(nil), ctx)
        XCTAssertTrue(opened().isEmpty)
    }
}
