import XCTest
import UIKit
import MetamonRenderer
@testable import Metamon

/// #627: 送る時に入力欄(UIKit)から全文を読み、入力欄そのものを空にする。
@MainActor
final class FocusedTextSubmissionTests: XCTestCase {
    private var window: UIWindow?
    private var previousKey: UIWindow?

    override func tearDown() async throws {
        await MainActor.run {
            window?.isHidden = true
            window = nil
            previousKey?.makeKey()
        }
    }

    private func present(_ input: UIView) throws {
        let scene = try XCTUnwrap(UIApplication.shared.connectedScenes.compactMap { $0 as? UIWindowScene }.first)
        previousKey = scene.keyWindow
        let w = UIWindow(windowScene: scene)
        let vc = UIViewController()
        w.rootViewController = vc
        input.frame = CGRect(x: 0, y: 100, width: 300, height: 44)
        vc.view.addSubview(input)
        w.makeKeyAndVisible()
        window = w
        XCTAssertTrue(input.becomeFirstResponder())
    }

    func testTakeAndClearTextView() throws {
        let view = UITextView()
        try present(view)
        view.text = "買い物メモ"
        XCTAssertEqual(KeyboardState.takeFocusedText(), "買い物メモ")
        KeyboardState.clearFocusedText()
        XCTAssertEqual(view.text, "")
    }

    /// 変換中(marked)の文字も確定させて含める。
    func testMarkedTextIsCommittedAndIncluded() throws {
        let field = UITextField()
        try present(field)
        field.text = "今日の"
        field.setMarkedText("よてい", selectedRange: NSRange(location: 3, length: 0))
        XCTAssertEqual(KeyboardState.takeFocusedText(), "今日のよてい")
        XCTAssertNil(field.markedTextRange)
        KeyboardState.clearFocusedText()
        XCTAssertEqual(field.text, "")
    }

    func testNoFocusedInputReturnsNil() {
        KeyboardState.dismiss()
        XCTAssertNil(KeyboardState.takeFocusedText())
    }
}
