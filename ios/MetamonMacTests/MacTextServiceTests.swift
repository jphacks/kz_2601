import XCTest
import AppKit
@testable import Metamon

/// サービスメニューの「Tailor It に送る」(#1849)
@MainActor
final class MacTextServiceTests: XCTestCase {
    private var pasteboards: [NSPasteboard] = []

    override func tearDown() {
        pasteboards.forEach { $0.releaseGlobally() }
        pasteboards = []
        super.tearDown()
    }

    private func pasteboard(_ text: String?) -> NSPasteboard {
        let pboard = NSPasteboard.withUniqueName()
        pasteboards.append(pboard)
        pboard.clearContents()
        if let text { pboard.setString(text, forType: .string) }
        return pboard
    }

    private func invoke(_ pboard: NSPasteboard) -> (routed: [String], error: NSString?) {
        var routed: [String] = []
        let provider = MacTextServiceProvider { routed.append($0) }
        var error: NSString?
        provider.sendToTailorIt(pboard, userData: nil, error: &error)
        return (routed, error)
    }

    func testTextIsRoutedToCompose() {
        let result = invoke(pasteboard("  牛乳を買う\n"))
        XCTAssertEqual(result.routed, ["牛乳を買う"], "前後の空白を除いて会話の入力欄へ")
        XCTAssertNil(result.error)
    }

    func testEmptyTextReturnsError() {
        for text in ["", "  \n\t", nil] as [String?] {
            let result = invoke(pasteboard(text))
            XCTAssertTrue(result.routed.isEmpty, "空の時は会話を開かない")
            XCTAssertNotNil(result.error, "空の時はサービスにエラーを返す")
        }
    }

    func testRegisteredSelectorMatchesInfoPlist() throws {
        let services = try XCTUnwrap(Bundle.main.object(forInfoDictionaryKey: "NSServices") as? [[String: Any]])
        let message = try XCTUnwrap(services.first?["NSMessage"] as? String)
        XCTAssertEqual(message, "sendToTailorIt")
        let selector = NSSelectorFromString("\(message):userData:error:")
        XCTAssertTrue(MacTextServiceProvider.shared.responds(to: selector), "NSMessage と同じ名前のメソッドがある")
    }

    func testAppendingKeepsDraft() {
        XCTAssertEqual(MacTextServiceProvider.appending("b", to: ""), "b")
        XCTAssertEqual(MacTextServiceProvider.appending("b", to: " \n"), "b")
        XCTAssertEqual(MacTextServiceProvider.appending("b", to: "a\n"), "a\nb", "書きかけは消さずに次の行へ")
    }
}
