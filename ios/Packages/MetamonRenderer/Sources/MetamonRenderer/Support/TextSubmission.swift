import Foundation
#if canImport(UIKit)
import UIKit
#else
import AppKit
#endif
import MetamonDSL

/// 入力欄の文字を「送る」時の共通処理(#627)。
///
/// 送信の後に文字が残る原因は 2 つ:
/// - SwiftUI のバインディングには、日本語の変換中(未確定)の文字がまだ入っていない。確定はキーボードとの往復で遅れて届く
/// - バインディングだけ空にしても、キーボードは送る前の文書を覚えていて、確定・予測変換の確定が後から入力欄へ書き戻される
///
/// そこで、送る時は入力欄(UIKit)で確定させてその場の全文を読み、入力欄そのものを空にしてキーボードに文書が空になったと伝える。
/// それでも直後に書き戻された「送った文字の一部」は `SentTextEcho` で消す。
public enum TextSubmission {
    /// 送る文字(前後の空白・改行を除く)。空なら nil。
    public static func value(_ text: String) -> String? {
        let v = text.trimmingCharacters(in: .whitespacesAndNewlines)
        return v.isEmpty ? nil : v
    }

    /// 送る前に入力欄の内容を確定させる必要があるアクション(フォームを読む・空にする)。
    public static func commitsInput(before action: ActionType?) -> Bool {
        switch action {
        case .saveRecord?, .resetForm?, .lookupBook?: true
        default: false
        }
    }
}

/// 送った直後に、キーボードから送った文字(またはその末尾)が書き戻されたかの判定(#627)。
public struct SentTextEcho: Equatable {
    public let sent: String
    public let at: Date
    /// 書き戻しを待つ時間。人が次の文字を打ち始めるより十分短い。
    public static let window: TimeInterval = 0.6

    public init(sent: String, at: Date = Date()) {
        self.sent = sent
        self.at = at
    }

    /// `text` に変わった時に空へ戻すか。送った文字そのもの・その末尾(確定が遅れて届いた部分)だけを消す。
    public func shouldClear(_ text: String, now: Date = Date()) -> Bool {
        guard now.timeIntervalSince(at) <= Self.window, let v = TextSubmission.value(text) else { return false }
        return sent.hasSuffix(v)
    }
}

extension KeyboardState {
    /// 今フォーカスのある入力欄で変換中の文字を確定させ、その全文を返す(入力欄が無ければ nil)。
    /// バインディングは確定より遅れて更新されるので、送る文字は入力欄から直接読む。
    public static func takeFocusedText() -> String? {
        #if os(macOS)
        guard let text = focusedTextView() else { return nil }
        text.unmarkText()
        return text.string
        #else
        guard let input = focusedTextInput() else { return nil }
        input.unmarkText()
        guard let range = input.textRange(from: input.beginningOfDocument, to: input.endOfDocument) else { return nil }
        return input.text(in: range)
        #endif
    }

    /// 今フォーカスのある入力欄を空にする。編集として入れるので、キーボードの予測・変換の文脈も空になる。
    public static func clearFocusedText() {
        #if os(macOS)
        guard let text = focusedTextView(), !text.string.isEmpty else { return }
        text.unmarkText()
        text.selectAll(nil)
        text.insertText("", replacementRange: NSRange(location: 0, length: (text.string as NSString).length))
        #else
        guard let input = focusedTextInput() else { return }
        input.unmarkText()
        guard let range = input.textRange(from: input.beginningOfDocument, to: input.endOfDocument),
              !range.isEmpty else { return }
        input.replace(range, withText: "")
        #endif
    }

    /// フォームを読む前: 変換中の文字を確定させてキーボードを閉じ、確定した文字がバインディングに届くのを 1 ループ待つ。
    /// キーボードを閉じると入力の途中(変換・音声入力)が終わるので、保存やリセットの後に書き戻されない。
    public static func finishEditing() async {
        #if os(macOS)
        guard focusedTextView() != nil else { return }
        #else
        guard focusedTextInput() != nil else { return }
        #endif
        commitMarkedText()
        dismiss()
        await withCheckedContinuation { (c: CheckedContinuation<Void, Never>) in
            DispatchQueue.main.async { c.resume() }
        }
    }

    #if os(macOS)
    /// 今フォーカスのある入力欄(TextField はキーウィンドウのフィールドエディタ・TextEditor は NSTextView そのもの)。
    static func focusedTextView() -> NSTextView? {
        NSApp?.keyWindow?.firstResponder as? NSTextView
    }
    #else
    private static func focusedTextInput() -> (UIView & UITextInput)? {
        let windows = UIApplication.shared.connectedScenes.compactMap { ($0 as? UIWindowScene)?.keyWindow }
        for window in windows {
            if let found = firstResponderInput(in: window) { return found }
        }
        return nil
    }

    private static func firstResponderInput(in view: UIView) -> (UIView & UITextInput)? {
        if view.isFirstResponder {
            if let field = view as? UITextField { return field }
            if let text = view as? UITextView { return text }
        }
        for sub in view.subviews {
            if let found = firstResponderInput(in: sub) { return found }
        }
        return nil
    }
    #endif
}
