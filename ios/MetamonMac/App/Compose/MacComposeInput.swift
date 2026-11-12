import AppKit
import MetamonRenderer

/// 会話の入力欄で送る前の確定(#1047)。日本語の変換中(未確定)の文字は、送るボタン・声の文を足す時に
/// 先に確定させ、欄の全文を下書きにする(バインディングには確定が遅れて届くので、欄から直接読む)。
@MainActor
enum MacComposeInput {
    /// 入力欄(`editor`)があれば変換中の文字を確定させた全文、無ければ今の下書き
    static func committedDraft(_ draft: String, editor: NSTextView?) -> String {
        editor?.commitMarkedTextNow() ?? draft
    }

    /// フォーカスのある入力欄で確定させた下書き。入力欄にフォーカスが無ければ今の下書きのまま
    static func committedDraft(_ draft: String, inputFocused: Bool) -> String {
        guard inputFocused else { return draft }
        return KeyboardState.takeFocusedText() ?? draft
    }

    /// Return で送ってよいか。変換中の Return は変換の確定なので送らない
    static func allowsReturnSubmit() -> Bool {
        KeyboardState.allowsReturnSubmit()
    }
}
