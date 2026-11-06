import SwiftUI
import MetamonRenderer

/// 画面下に固定する文字入力バー(#196・LINE のトーク入力が手本)。音声シートの初期画面と対話で共通。
/// - 置き場所は呼び出し側の `.safeAreaInset(edge: .bottom)`。キーボードと同じアニメーションで一緒に上がる(手動のオフセット計算はしない)
/// - 1 行から文字に合わせて伸び、6 行を超えたら欄の中でスクロール。return は改行、送信は送信ボタン
/// - 送信の前に日本語の変換中の文字を確定させ、送った後は入力欄が必ず空になる(#627・`TextSubmission`)
struct ChatComposer<Leading: View>: View {
    let placeholder: String
    @Binding var text: String
    var focused: FocusState<Bool>.Binding
    var disabled = false
    /// 送信後もキーボードを出したままにするか(対話は続けて打てるよう true)
    var keepsFocus = true
    let onSend: (String) -> Void
    @ViewBuilder var leading: Leading
    @State private var echo: SentTextEcho?

    static var fieldMinHeight: CGFloat { 44 }

    private var canSend: Bool { !text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty && !disabled }

    var body: some View {
        HStack(alignment: .bottom, spacing: Theme.Spacing.sm) {
            leading
            TextField(placeholder, text: $text, axis: .vertical)
                .lineLimit(1 ... 6)
                .font(.body)
                .focused(focused)
                // 左右と上下の余白を揃え、1 行のとき高さ 44(= 送信ボタン)になるようにする
                .padding(.horizontal, ComposerMetrics.inset)
                .padding(.vertical, ComposerMetrics.verticalInset)
                .frame(minHeight: Self.fieldMinHeight)
                .background(Theme.Colors.surface, in: RoundedRectangle(cornerRadius: ComposerMetrics.radius, style: .continuous))
                .overlay(RoundedRectangle(cornerRadius: ComposerMetrics.radius, style: .continuous)
                    .strokeBorder(focused.wrappedValue ? Theme.Colors.brandA.opacity(0.6) : Theme.Colors.hairline))
                .accessibilityIdentifier("composer.field")
                .onChange(of: text) { _, new in dropEcho(new) }
            Button(action: send) {
                Image(systemName: "arrow.up")
                    .font(.body.weight(.bold))
                    .foregroundStyle(.white)
                    .frame(width: Self.fieldMinHeight, height: Self.fieldMinHeight)
                    .background(canSend ? AnyShapeStyle(Theme.brandGradient) : AnyShapeStyle(Color.gray.opacity(0.4)), in: Circle())
            }
            .disabled(!canSend)
            .accessibilityLabel("送信")
            .accessibilityIdentifier("composer.send")
        }
    }

    private func send() {
        // 入力欄で変換中の文字を確定させ、その場の全文を読む(バインディングは確定より遅れる・#627)
        let raw = KeyboardState.takeFocusedText() ?? text
        guard let value = TextSubmission.value(raw), !disabled else { return }
        // 入力欄そのものを空にしてキーボードの文書も空にする(バインディングだけ空にすると後から書き戻される)
        KeyboardState.clearFocusedText()
        text = ""
        echo = SentTextEcho(sent: value)
        if !keepsFocus { focused.wrappedValue = false }
        onSend(value)
    }

    /// 送った直後にキーボードから書き戻された文字(送った文字の末尾)を消す(#627)。
    private func dropEcho(_ new: String) {
        guard let echo else { return }
        if echo.shouldClear(new) {
            KeyboardState.clearFocusedText()
            text = ""
        } else if !new.isEmpty {
            self.echo = nil // 新しく打ち始めた
        }
    }
}

extension ChatComposer where Leading == EmptyView {
    init(placeholder: String, text: Binding<String>, focused: FocusState<Bool>.Binding, disabled: Bool = false,
         keepsFocus: Bool = true, onSend: @escaping (String) -> Void) {
        self.init(placeholder: placeholder, text: text, focused: focused, disabled: disabled, keepsFocus: keepsFocus,
                  onSend: onSend, leading: { EmptyView() })
    }
}

/// 入力欄の余白・角丸(本体のトークン。Renderer の inputH/inputV と同じ値)。
enum ComposerMetrics {
    static let inset: CGFloat = 14
    /// body 1 行(約 22pt)+ 上下 11 = 44
    static let verticalInset: CGFloat = 11
    static let radius: CGFloat = 22
}

extension View {
    /// 本体のフォーム(シェア・リリース申請など)の入力欄の器。左右 14・上下 12 の均等な余白 + 角丸 + 細い枠。
    func metamonInputField() -> some View {
        self
            .textFieldStyle(.plain)
            .padding(.horizontal, ComposerMetrics.inset)
            .padding(.vertical, 12)
            .background(Theme.Colors.surfaceElevated, in: RoundedRectangle(cornerRadius: Theme.Radius.sm + 2, style: .continuous))
            .overlay(RoundedRectangle(cornerRadius: Theme.Radius.sm + 2, style: .continuous).strokeBorder(Theme.Colors.hairline))
    }
}
