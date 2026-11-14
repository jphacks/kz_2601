import XCTest
import AppKit
import MetamonDSL
@testable import MetamonRenderer
import MetamonServices
@testable import Metamon

/// 保存していない入力がある時のウインドウを閉じる・アプリを終える前の確認(`MacDiscardGuard`・#943 / #2324)。
/// 「破棄」= `discardRequest.perform()`、「やめる」= `discardRequest = nil`(`AppCanvas` のダイアログと同じ)。
@MainActor
final class MacDiscardGuardTests: XCTestCase {
    private let home = Route(screen: "home", stack: Route.tabStack("home"))
    private let formSheet = Route(screen: "expense_form", stack: "sheet")

    private func runtime() throws -> MacAppRuntime {
        let url = try XCTUnwrap(Bundle(for: MacAppRuntime.self).url(forResource: "kakeibo", withExtension: "json"))
        return MacAppRuntime(appId: "test-mac-\(UUID().uuidString.lowercased())", dsl: try AppDSL.load(contentsOf: url),
                             ai: MockAIClient(delay: .zero), isAIConsentGranted: { false })
    }

    /// 支出のフォームをシートで開き、`ScreenView` と同じく控えに入れる
    private func openForm(_ rt: MacAppRuntime) async throws -> FormDraft {
        let open = try JSONDecoder().decode(Action.self, from: Data(#"{"type":"navigate","screen":"expense_form","presentation":"sheet"}"#.utf8))
        await rt.run(open, ActionContext(route: home))
        XCTAssertEqual(rt.sheet, formSheet)
        let form = FormDraft(entity: try XCTUnwrap(rt.dsl.entity("expense")))
        rt.openForms.track(form, at: formSheet)
        return form
    }

    /// 控えはフォームを弱く持つので、テストの間はここで持つ(`ScreenView` が持つ代わり)
    private var forms: [FormDraft] = []

    override func tearDown() async throws {
        forms = []
    }

    /// 入力のあるフォームを開いた見張り
    private func guardWithInput() async throws -> (MacDiscardGuard, MacAppRuntime) {
        let rt = try runtime()
        let form = try await openForm(rt)
        form.values["memo"] = .string("コンビニ")
        forms.append(form)
        let discardGuard = MacDiscardGuard()
        discardGuard.runtime = rt
        return (discardGuard, rt)
    }

    // MARK: - ⌘W・閉じるボタン

    func testClosesWithoutAskingWhenNoInput() async throws {
        let rt = try runtime()
        _ = try await openForm(rt) // 開いただけ
        let discardGuard = MacDiscardGuard()
        discardGuard.runtime = rt
        var closed = false
        XCTAssertFalse(discardGuard.hasUnsavedInput)
        XCTAssertTrue(discardGuard.allowsClose { closed = true }, "入力が無ければそのまま閉じる")
        XCTAssertNil(rt.discardRequest, "確認を出さない")
        XCTAssertFalse(closed, "閉じ直しは呼ばない(そのまま閉じるので)")
    }

    func testClosesWithoutRuntime() {
        let discardGuard = MacDiscardGuard()
        XCTAssertFalse(discardGuard.hasUnsavedInput)
        XCTAssertTrue(discardGuard.allowsClose {}, "アプリを開いていないウインドウは聞かない")
    }

    func testAsksBeforeClosingWithInputAndDiscardCloses() async throws {
        let (discardGuard, rt) = try await guardWithInput()
        var closed = false
        XCTAssertTrue(discardGuard.hasUnsavedInput)
        XCTAssertFalse(discardGuard.allowsClose { closed = true }, "入力があれば止めて確認する")
        let request = try XCTUnwrap(rt.discardRequest)
        XCTAssertFalse(closed)
        rt.discardRequest = nil
        request.perform() // 破棄
        XCTAssertTrue(closed, "「破棄」で閉じ直す")
    }

    func testCancelKeepsWindowOpen() async throws {
        let (discardGuard, rt) = try await guardWithInput()
        var closed = false
        XCTAssertFalse(discardGuard.allowsClose { closed = true })
        rt.discardRequest = nil // やめる
        XCTAssertFalse(closed, "「やめる」では閉じない")
        XCTAssertTrue(discardGuard.hasUnsavedInput, "入力は残っている")
    }

    func testReleasedDoesNotAskAgain() async throws {
        let (discardGuard, rt) = try await guardWithInput()
        discardGuard.released = true
        XCTAssertFalse(discardGuard.hasUnsavedInput, "「破棄」を選んだ後(シートを閉じるまで)は聞き直さない")
        XCTAssertTrue(discardGuard.allowsClose {})
        XCTAssertNil(rt.discardRequest)
    }

    // MARK: - アプリの切り替え(run)

    func testRunActsAtOnceWithoutInput() throws {
        let rt = try runtime()
        let discardGuard = MacDiscardGuard()
        discardGuard.runtime = rt
        var asked = false
        discardGuard.beforeAsking = { asked = true }
        var done = false
        discardGuard.run { done = true }
        XCTAssertTrue(done)
        XCTAssertFalse(asked, "確認を出さないのでシートも下げない")
        XCTAssertNil(rt.discardRequest)
    }

    func testRunAsksWithInput() async throws {
        let (discardGuard, rt) = try await guardWithInput()
        var asked = false
        discardGuard.beforeAsking = { asked = true }
        var done = false
        discardGuard.run { done = true }
        XCTAssertFalse(done, "確認の答えを待つ")
        XCTAssertTrue(asked, "確認の前にシェルのシートを下げる")
        let request = try XCTUnwrap(rt.discardRequest)
        request.perform()
        XCTAssertTrue(done, "「破棄」で進める")
    }

    func testRunWhenReleasedActsAtOnce() async throws {
        let (discardGuard, rt) = try await guardWithInput()
        discardGuard.released = true
        var done = false
        discardGuard.run { done = true }
        XCTAssertTrue(done)
        XCTAssertNil(rt.discardRequest)
    }

    // MARK: - ⌘Q

    func testQuitWithoutInputDoesNotAsk() throws {
        let rt = try runtime()
        let discardGuard = MacDiscardGuard()
        discardGuard.runtime = rt
        var quit = false
        XCTAssertFalse(MacDiscardGuard.confirmQuit { quit = true }, "入力が無ければ聞かずに終える")
        XCTAssertNil(rt.discardRequest)
        XCTAssertFalse(quit)
        XCTAssertEqual(MacAppDelegate().applicationShouldTerminate(NSApp), .terminateNow)
        withExtendedLifetime(discardGuard) {}
    }

    func testQuitWithInputAsksAndDiscardQuits() async throws {
        let (discardGuard, rt) = try await guardWithInput()
        var asked = false
        discardGuard.beforeAsking = { asked = true }
        var quit = false
        XCTAssertTrue(MacDiscardGuard.confirmQuit { quit = true }, "入力があれば終了を止めて確認する")
        XCTAssertTrue(asked)
        let request = try XCTUnwrap(rt.discardRequest)
        XCTAssertFalse(quit)
        rt.discardRequest = nil
        request.perform()
        XCTAssertTrue(quit, "「破棄」で終える")
    }

    func testQuitCancelKeepsRunning() async throws {
        let (discardGuard, rt) = try await guardWithInput()
        XCTAssertEqual(MacAppDelegate().applicationShouldTerminate(NSApp), .terminateCancel, "⌘Q をいったん止める")
        XCTAssertNotNil(rt.discardRequest)
        rt.discardRequest = nil // やめる(perform は呼ばない = 終了しない)
        XCTAssertTrue(discardGuard.hasUnsavedInput)
    }

    // MARK: - ウインドウの delegate に挟む部分

    func testDelegateStopsCloseAndClosesAgainWithoutAsking() throws {
        let window = NSWindow(contentRect: NSRect(x: 0, y: 0, width: 200, height: 100),
                              styleMask: [.titled, .closable], backing: .buffered, defer: true)
        window.isReleasedWhenClosed = false
        let original = OriginalDelegate()
        var asks = 0
        var closeAgain: (() -> Void)?
        let proxy = CloseGuardDelegate(original: original) { again in
            asks += 1
            closeAgain = again
            return false
        }
        window.delegate = proxy

        XCTAssertFalse(proxy.windowShouldClose(window), "入力があれば閉じない")
        XCTAssertEqual(asks, 1)
        XCTAssertEqual(original.shouldCloseCalls, 0, "元の delegate には渡さない")

        try XCTUnwrap(closeAgain)() // 「破棄」
        XCTAssertEqual(asks, 1, "閉じ直しではもう聞かない")
        XCTAssertEqual(original.shouldCloseCalls, 1, "閉じ直しは元の delegate へ渡す")

        XCTAssertFalse(proxy.windowShouldClose(window), "次に閉じる時はまた聞く")
        XCTAssertEqual(asks, 2)
        withExtendedLifetime(proxy) {}
    }

    func testDelegatePassesThroughWhenAllowed() {
        let window = NSWindow(contentRect: .zero, styleMask: [.titled, .closable], backing: .buffered, defer: true)
        window.isReleasedWhenClosed = false
        let original = OriginalDelegate()
        let proxy = CloseGuardDelegate(original: original) { _ in true }
        XCTAssertTrue(proxy.windowShouldClose(window))
        XCTAssertEqual(original.shouldCloseCalls, 1)
        original.allowsClose = false
        XCTAssertFalse(proxy.windowShouldClose(window), "元の delegate の答えに従う")
    }

    func testDelegateForwardsOtherCallsToOriginal() {
        let original = OriginalDelegate()
        let proxy = CloseGuardDelegate(original: original) { _ in true }
        let resize = #selector(NSWindowDelegate.windowDidResize(_:))
        XCTAssertTrue(proxy.responds(to: resize), "SwiftUI の delegate の他の呼び出しを残す")
        XCTAssertTrue(proxy.forwardingTarget(for: resize) as AnyObject === original)
        let move = #selector(NSWindowDelegate.windowDidMove(_:))
        XCTAssertFalse(proxy.responds(to: move), "元が持たないものは持たない")
    }
}

@MainActor
private final class OriginalDelegate: NSObject, NSWindowDelegate {
    var shouldCloseCalls = 0
    var allowsClose = true

    func windowShouldClose(_ sender: NSWindow) -> Bool {
        shouldCloseCalls += 1
        return allowsClose
    }

    func windowDidResize(_ notification: Notification) {}
}
