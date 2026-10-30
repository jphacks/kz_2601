#if !APP_STORE_REVIEW
// 審査用ビルド(APP_STORE_REVIEW・#245)では登録しない(未知 type のプレースホルダになる)。
import SwiftUI
import MetamonDSL

/// `audioRecorder`(#420): 録音・停止・再生・録り直し・削除。録音中は経過時間 / 上限と音量メーター。
/// 録音は端末内に保存し、field には `"audio:<id>|<秒>"`(`AudioValue`)を入れる。
/// 画面にある間はイヤホンの 再生 / 一時停止 ボタンで録音を開始・停止できる(`RemoteRecorderCommands`)。
struct AudioRecorderLeaf: View {
    let c: Component
    let ctx: RenderContext

    var body: some View {
        if let form = ctx.form, let field = c.field {
            AudioRecorderPanel(c: c, form: form, field: field)
        } else {
            NeedsForm(c: c)
        }
    }
}

struct AudioRecorderPanel: View {
    let c: Component
    let form: FormDraft
    let field: String

    @State private var recorder: AudioRecorderController
    @State private var showDenied = false
    @Environment(\.openURL) private var openURL
    private var playback: AudioPlayback { .shared }

    init(c: Component, form: FormDraft, field: String) {
        self.c = c
        self.form = form
        self.field = field
        _recorder = State(initialValue: AudioRecorderController(maxDuration: c.maxDuration, hasClip: AudioValue(form.values[field] ?? .null) != nil))
    }

    private var value: AudioValue? { AudioValue(form.values[field] ?? .null) }
    private var title: String { c.label.flatMap { $0.isEmpty ? nil : $0 } ?? L("録音") }

    var body: some View {
        VStack(alignment: .leading, spacing: RenderTheme.Spacing.sm) {
            Text(title)
                .font(RenderTheme.Fonts.inputLabel)
                .foregroundStyle(.secondary)
                .padding(.leading, 2)
            if recorder.isRecording {
                recording
            } else if let value {
                recorded(value)
            } else {
                idle
            }
            if recorder.failure == .failed {
                Text(L("録音できませんでした。もう一度お試しください。")).font(.caption).foregroundStyle(.red)
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("audioRecorder.\(field)")
        .onAppear {
            recorder.onRecorded = { [form, field] v in form.values[field] = .string(v.encoded) }
            RemoteRecorderCommands.shared.attach(owner: recorder, title: title) { [recorder] command in
                handle(command, recorder: recorder)
            }
        }
        .onDisappear {
            recorder.stop() // 録れた分は残す(フォームを閉じたら保存はされない)
            if let id = value?.id, playback.isPlaying(id) { playback.stop() }
            RemoteRecorderCommands.shared.detach(owner: recorder)
        }
        .onChange(of: value?.id) { _, _ in recorder.sync(hasClip: value != nil) }
        .onChange(of: recorder.failure) { _, f in if f == .denied { showDenied = true } }
        .alert(L("マイクを使えません"), isPresented: $showDenied) {
            Button(L("設定を開く")) {
                if let url = URL(string: UIApplication.openSettingsURLString) { openURL(url) }
            }
            Button(L("キャンセル"), role: .cancel) {}
        } message: {
            Text(L("設定アプリで Tailor It のマイクへのアクセスを許可してください。"))
        }
    }

    // MARK: - states

    private var idle: some View {
        VStack(spacing: RenderTheme.Spacing.xs) {
            Button {
                Task { await recorder.start() }
            } label: {
                Label(L("録音する"), systemImage: "mic.fill").frame(maxWidth: .infinity)
            }
            .buttonStyle(RenderButtonStyle(kind: .primary))
            .accessibilityIdentifier("audioRecorder.record")
            Text(L("上限 \(AudioRecorderPolicy.elapsedText(Double(recorder.maxDuration)))・イヤホンのボタンでも開始・停止できます"))
                .font(.caption2)
                .foregroundStyle(.tertiary)
                .frame(maxWidth: .infinity)
        }
    }

    private var recording: some View {
        VStack(spacing: RenderTheme.Spacing.sm) {
            HStack(alignment: .firstTextBaseline, spacing: 6) {
                Circle().fill(.red).frame(width: 10, height: 10).accessibilityHidden(true)
                Text(AudioRecorderPolicy.elapsedText(recorder.elapsed))
                    .font(RenderTheme.Fonts.number)
                    .contentTransition(.numericText())
                Text("/ " + AudioRecorderPolicy.elapsedText(Double(recorder.maxDuration)))
                    .font(.subheadline.monospacedDigit())
                    .foregroundStyle(.secondary)
            }
            .accessibilityElement(children: .ignore)
            .accessibilityLabel(L("録音中 \(AudioRecorderPolicy.elapsedText(recorder.elapsed))"))
            AudioLevelMeter(level: recorder.level)
            Button(role: .destructive) {
                recorder.stop()
            } label: {
                Label(L("停止"), systemImage: "stop.fill").frame(maxWidth: .infinity)
            }
            .buttonStyle(RenderButtonStyle(kind: .destructive))
            .accessibilityIdentifier("audioRecorder.stop")
        }
        .frame(maxWidth: .infinity)
    }

    private func recorded(_ v: AudioValue) -> some View {
        VStack(spacing: RenderTheme.Spacing.sm) {
            HStack {
                AudioPlayButton(value: .string(v.encoded))
                Spacer()
                if !LocalAudioStore.shared.exists(v.id) {
                    Label(L("この端末にはありません"), systemImage: "iphone.slash")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
            }
            .padding(.horizontal, RenderTheme.Spacing.sm)
            .frame(minHeight: 44)
            .background(RenderTheme.Colors.brandSoft, in: RoundedRectangle(cornerRadius: RenderTheme.Radius.inner, style: .continuous))
            HStack(spacing: RenderTheme.Spacing.sm) {
                Button {
                    Task { await recorder.start() }
                } label: {
                    Label(L("録り直す"), systemImage: "mic").frame(maxWidth: .infinity)
                }
                .buttonStyle(RenderButtonStyle(kind: .secondary))
                .accessibilityIdentifier("audioRecorder.rerecord")
                Button(role: .destructive) {
                    recorder.discard()
                    form.values[field] = .null
                } label: {
                    Label(L("削除"), systemImage: "trash").frame(maxWidth: .infinity)
                }
                .buttonStyle(RenderButtonStyle(kind: .destructive))
                .accessibilityLabel(L("\(title)を削除"))
            }
        }
    }

    // MARK: - earphone buttons

    private func handle(_ command: AudioRecorderMachine.RemoteCommand, recorder: AudioRecorderController) {
        let playing = value.map { playback.isPlaying($0.id) } ?? false
        switch recorder.machine.remoteAction(command, isPlaying: playing) {
        case .startRecording: Task { await recorder.start() }
        case .stopRecording: recorder.stop()
        case .stopPlayback: playback.stop()
        case nil: break
        }
    }
}

/// 録音中の音量メーター(0...1)。細い棒 20 本・音量で左から点く。
struct AudioLevelMeter: View {
    let level: Double
    private let bars = 20

    var body: some View {
        HStack(spacing: 3) {
            ForEach(0..<bars, id: \.self) { i in
                Capsule()
                    .fill(Double(i) / Double(bars) < level ? RenderTheme.Colors.brandA : Color.secondary.opacity(0.2))
                    .frame(height: 6 + CGFloat(i % 4) * 3)
            }
        }
        .frame(height: 16)
        .animation(.linear(duration: 0.1), value: level)
        .accessibilityHidden(true)
    }
}
#endif
