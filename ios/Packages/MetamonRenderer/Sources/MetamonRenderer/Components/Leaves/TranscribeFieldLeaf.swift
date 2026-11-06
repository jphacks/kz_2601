import SwiftUI
import MetamonDSL

/// `transcribeField`: 押して話す → 端末の中で文字にして string / text の field の後ろに足す(複数行の欄はそのまま手で直せる)。
/// 声を文字にするのはホスト(`DictationHost`・アプリの音声入力と同じモデル)。声は端末の外に出さず、録音も残さない。
/// 画面を離れた・マイク FAB の音声入力が始まったら録音を捨てる(マイクを取り合わない)。
struct TranscribeFieldLeaf: View {
    let c: Component
    let ctx: RenderContext

    var body: some View {
        if let form = ctx.form, let field = c.field {
            TranscribeFieldPanel(c: c, form: form, field: field)
        } else {
            NeedsForm(c: c)
        }
    }
}

/// 録音の段階。
enum DictationPhase: Equatable {
    case idle, listening, transcribing
}

/// 文字起こしの結果を今の文の後ろに足す(空なら置き換え、それ以外は改行で区切る)。
enum DictationText {
    static func appending(_ new: String, to existing: String) -> String {
        let new = new.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !new.isEmpty else { return existing }
        let trimmed = existing.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return new }
        return existing.hasSuffix("\n") ? existing + new : existing + "\n" + new
    }
}

struct TranscribeFieldPanel: View {
    @Environment(\.dslRuntime) private var rt
    let c: Component
    let form: FormDraft
    let field: String

    @State private var phase: DictationPhase = .idle
    @State private var message: String?
    @FocusState private var focused: Bool

    private var host: (any DictationHost)? { rt.dictation }

    var body: some View {
        LabeledInput(label: c.label, focused: focused) {
            VStack(alignment: .leading, spacing: RenderTheme.Spacing.sm) {
                TextField(c.placeholder ?? L("マイクを押して話すと、ここに文字で入ります"), text: form.string(field), axis: .vertical)
                    .lineLimit(TextInputRules.editorLines(minLines: c.minLines))
                    .textFieldStyle(.plain)
                    .font(RenderTheme.Fonts.inputValue)
                    .focused($focused)
                    .keyboardAnchor(c, $focused)
                    .accessibilityIdentifier("field.\(field)")
                    .renderInputChrome(focused: focused)
                micRow
                if let message {
                    Text(message)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                        .accessibilityIdentifier("transcribeField.message")
                }
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("transcribeField.\(field)")
        .onDisappear { cancel() }
        .onChange(of: host?.isVoiceInputActive ?? false) { _, active in
            if active { cancel() }
        }
    }

    @ViewBuilder
    private var micRow: some View {
        if let host {
            switch (phase, host.dictationReadiness) {
            case (.listening, _):
                micButton(L("聞いています… タップで終わる"), icon: "stop.circle.fill", kind: .primary) { Task { await finish() } }
            case (.transcribing, _):
                HStack(spacing: RenderTheme.Spacing.sm) {
                    ProgressView()
                    Text(L("文字にしています…")).font(.subheadline).foregroundStyle(.secondary)
                }
                .frame(maxWidth: .infinity, minHeight: 44)
            case (.idle, .ready):
                micButton(L("話して入力"), icon: "mic.fill", kind: .secondary) { Task { await start() } }
            case (.idle, .preparing):
                HStack(spacing: RenderTheme.Spacing.sm) {
                    ProgressView()
                    Text(L("音声モデルを準備しています…")).font(.subheadline).foregroundStyle(.secondary)
                }
                .frame(maxWidth: .infinity, minHeight: 44)
            case (.idle, .needsModel), (.idle, .failed):
                micButton(L("音声モデルを用意する"), icon: "arrow.down.circle", kind: .secondary) { host.prepareDictation() }
            }
        } else {
            Text(L("この画面では話して入力を使えません"))
                .font(.caption)
                .foregroundStyle(.secondary)
        }
    }

    private func micButton(_ title: String, icon: String, kind: RenderButtonStyle.Kind, action: @escaping () -> Void) -> some View {
        Button(action: action) {
            Label(title, systemImage: icon).frame(maxWidth: .infinity)
        }
        .buttonStyle(RenderButtonStyle(kind: kind))
        .accessibilityIdentifier("transcribeField.mic")
    }

    private func start() async {
        guard let host, phase == .idle else { return }
        focused = false
        message = nil
        do {
            try await host.startDictation()
            phase = .listening
        } catch {
            message = error.localizedDescription
        }
    }

    private func finish() async {
        guard let host, phase == .listening else { return }
        phase = .transcribing
        do {
            let text = try await host.finishDictation()
            let current = (form.values[field] ?? .null).displayString
            form.values[field] = .string(DictationText.appending(text, to: current))
        } catch {
            message = error.localizedDescription
        }
        phase = .idle
    }

    private func cancel() {
        guard phase == .listening else { return }
        host?.cancelDictation()
        phase = .idle
    }
}
