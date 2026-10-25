import SwiftUI
import UIKit
import MetamonDSL

/// テキスト入力の共通ルール(#196・LINE のトーク入力が手本)。
/// - 複数行: `TextField(axis: .vertical)` で文字に合わせて伸び、return は改行(送信・確定にしない)
/// - 1 行: return で次の入力欄へ進み、最後の欄ならキーボードを閉じる
public enum TextInputRules {
    /// 複数行にするか。DSL の `multiline` が最優先。無ければ field 型 `text`(長文)か、本文・メモ系の名前(一覧の見出し field は除く)。
    public static func isMultiline(component c: Component, field: Field?, titleField: String?) -> Bool {
        if let explicit = c.multiline { return explicit }
        guard let field else { return false }
        if field.type == .text { return true }
        guard field.type == .string, field.id != titleField else { return false }
        let hay = (field.id + " " + field.name + " " + (c.label ?? "")).lowercased()
        return longTextHints.contains { hay.contains($0) }
    }

    static let longTextHints = ["本文", "日記", "メモ", "詳細", "説明", "感想", "body", "memo", "note", "description", "comment", "detail"]

    /// textField の複数行の行数(最小〜最大)。最大を超えたら欄の中でスクロール。
    static let multilineLines: ClosedRange<Int> = 3 ... 8

    /// textEditor の行数。`minLines`(既定 4)から伸び、最低でも 10 行まで。
    static func editorLines(minLines: Int?) -> ClosedRange<Int> {
        let low = Swift.max(minLines ?? 4, 1)
        return low ... Swift.max(low + 6, 10)
    }
}

extension KeyboardState {
    /// 画面上で今の入力欄の次(上 → 下、左 → 右)にある入力欄へフォーカスを移す。無ければキーボードを閉じる。
    /// leaf はそれぞれ自分の @FocusState しか持たないので、UIKit の responder で横断する。
    public static func focusNext() {
        guard let window = UIApplication.shared.connectedScenes
            .compactMap({ ($0 as? UIWindowScene)?.keyWindow }).first else { return dismiss() }
        let located: [(view: UIView, frame: CGRect)] = textInputs(in: window).map { view in
            (view: view, frame: view.convert(view.bounds, to: window))
        }
        let inputs = located
            .filter { $0.frame.width > 1 && $0.frame.height > 1 }
            .sorted { a, b in readingOrder(a.frame, b.frame) }
        guard let current = inputs.firstIndex(where: { $0.view.isFirstResponder }),
              current + 1 < inputs.count else { return dismiss() }
        let next = inputs[current + 1].view
        // SwiftUI が return の後にフォーカスを外すことがあるので、次のループでも確かめる
        next.becomeFirstResponder()
        DispatchQueue.main.async { if !next.isFirstResponder { next.becomeFirstResponder() } }
    }

    /// 上 → 下、同じ行(4pt 以内)なら左 → 右。
    private static func readingOrder(_ a: CGRect, _ b: CGRect) -> Bool {
        let dy: CGFloat = a.minY - b.minY
        if abs(dy) > 4 { return dy < 0 }
        return a.minX < b.minX
    }

    private static func textInputs(in view: UIView) -> [UIView] {
        if view.isHidden || view.alpha < 0.01 || !view.isUserInteractionEnabled { return [] }
        if let field = view as? UITextField { return field.isEnabled ? [field] : [] }
        if let text = view as? UITextView { return text.isEditable ? [text] : [] }
        return view.subviews.flatMap(textInputs(in:))
    }

    /// 日本語の変換中(未確定の文字)を確定させる。送信ボタンで未確定の文字が消えたり二重に残ったりしないよう、読む前に呼ぶ。
    public static func commitMarkedText() {
        UIApplication.shared.sendAction(#selector(UITextInput.unmarkText), to: nil, from: nil, for: nil)
    }
}

extension View {
    /// 1 行の入力欄: return =「次へ」(最後の欄なら閉じる)。
    func singleLineSubmit() -> some View {
        submitLabel(.next)
            .onSubmit { KeyboardState.focusNext() }
    }
}
