import SwiftUI
import AppKit
import MetamonServices

/// 会話のシートの入力欄の横のマイク(#966)。押して録音・もう一度押して止める(⇧⌘M でも同じ)。
/// 長く押した時は、離したら止めて送る(#1220)。VoiceOver・キーボードの操作は押して開始・もう一度で終了。
struct MacMicButton: View {
    @Environment(MacVoiceInput.self) private var voice
    let disabled: Bool
    let deliver: (String) -> Void
    @State private var gesture = HoldToTalkGesture()

    private var isDisabled: Bool { voice.state == .transcribing || (disabled && voice.state == .idle) }

    var body: some View {
        label
            .frame(width: 24, height: 24)
            .contentShape(Rectangle())
            .opacity(isDisabled ? 0.4 : 1)
            .gesture(DragGesture(minimumDistance: 0)
                .onChanged { _ in
                    guard !gesture.isPressed, !isDisabled else { return }
                    run(gesture.press(.immediate, at: Date(), isRecording: voice.isRecording))
                }
                .onEnded { _ in run(gesture.release(at: Date())) })
            // フルキーボードアクセス: Tab で止まり、スペース・Return で押して開始・もう一度で終了(押している間の録音はポインタだけ)
            .focusable(!isDisabled, interactions: .activate)
            .onKeyPress(keys: [.space, .return]) { _ in
                guard !isDisabled else { return .ignored }
                voice.toggle(deliver: deliver)
                return .handled
            }
            .help(voice.isRecording ? Text("話し終えた(⇧⌘M)。長く押した時は離すと送ります")
                                    : Text("声で頼む(⇧⌘M)。押している間だけ話すこともできます"))
            .accessibilityElement()
            .accessibilityAddTraits(.isButton)
            .accessibilityLabel(voice.isRecording ? Text("話し終えた") : Text("声で頼む"))
            .accessibilityAction { if !isDisabled { voice.toggle(deliver: deliver) } }
            .accessibilityIdentifier("compose.mic")
    }

    @ViewBuilder private var label: some View {
        switch voice.state {
        case .idle:
            Image(systemName: "mic").font(.title3)
        case .recording:
            Image(systemName: "stop.circle.fill").font(.title2).foregroundStyle(.red)
                .symbolEffect(.pulse, options: .repeating)
        case .transcribing:
            ProgressView().controlSize(.small)
        }
    }

    private func run(_ action: HoldToTalkGesture.Action) {
        switch action {
        case .start: voice.holdStart(deliver: deliver)
        case .stop: voice.holdStop(deliver: deliver)
        case .none, .insertSpace: break
        }
    }
}

/// 入力欄の上に出す、声の状態(録音中・文字にしている・モデルの取得・失敗)。何も無ければ出さない。
struct MacVoiceStatus: View {
    @Environment(MacVoiceInput.self) private var voice
    @Environment(\.openURL) private var openURL

    /// 出すものがあるか(無ければ入力欄の上に余白を作らない)
    private var hasContent: Bool {
        if voice.state != .idle || voice.failure != nil { return true }
        switch voice.status {
        case .downloading, .loading: return true
        case .notDownloaded: return voice.isProvisional
        default: return false
        }
    }

    var body: some View {
        if hasContent { content }
    }

    private var content: some View {
        VStack(alignment: .leading, spacing: 6) {
            switch voice.state {
            case .recording:
                HStack(spacing: 6) {
                    Image(systemName: "waveform").foregroundStyle(.red)
                    Text(MacVoiceCopy.listening)
                }
                if voice.isProvisional { caption(MacVoiceCopy.provisional) }
            case .transcribing:
                HStack(spacing: 6) {
                    ProgressView().controlSize(.small)
                    Text(MacVoiceCopy.transcribing)
                }
            case .idle:
                if let failure = voice.failure {
                    Label(failure, systemImage: "exclamationmark.triangle")
                        .foregroundStyle(Theme.Colors.warning)
                        .fixedSize(horizontal: false, vertical: true)
                    if let pane = voice.settingsPane {
                        Button(MacVoiceCopy.openSettings) { openURL(pane.url) }
                            .controlSize(.small)
                    } else if voice.status == .notDownloaded {
                        downloadButton
                    }
                }
                modelStatus
            }
        }
        .font(.callout)
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(.horizontal, MacTheme.Spacing.window)
        .padding(.top, MacTheme.Spacing.group)
        .accessibilityElement(children: .contain)
    }

    @ViewBuilder private var modelStatus: some View {
        switch voice.status {
        case .downloading:
            ProgressView(value: voice.downloadProgress)
                .progressViewStyle(.linear)
                .tint(Theme.Colors.brandA)
            caption(MacVoiceCopy.downloading(percent: Int(voice.downloadProgress * 100)))
        case .loading:
            HStack(spacing: 6) {
                ProgressView().controlSize(.small)
                caption(MacVoiceCopy.loading)
            }
        case .notDownloaded where voice.failure == nil && voice.isProvisional:
            // 簡易の聞き取りで話せる。より正確なモデルを入れる入口だけ出す
            caption(MacVoiceCopy.provisionalTextFirst)
            downloadButton
        default:
            EmptyView()
        }
    }

    private var downloadButton: some View {
        Button(MacVoiceCopy.downloadButton) { voice.requestDownload() }
            .controlSize(.small)
    }

    private func caption(_ text: String) -> some View {
        Text(text).font(.caption).foregroundStyle(.secondary).fixedSize(horizontal: false, vertical: true)
    }
}

/// 音声モデルのダウンロードの確認(iOS の ConsentHost と同じ形・#246・#275)。会話のシートに付ける。
/// `shown`: 生成したアプリの話して入力(#1328)から出す確認の旗。中央の列に付ける(会話のシートとは別の旗で、2 か所に同時に出さない)。
struct MacModelDownloadConfirm: ViewModifier {
    /// 中央の列のシートの中では渡っていないことがあるので任意で読む(無ければ何も出さない)
    @Environment(MacVoiceInput.self) private var voice: MacVoiceInput?
    var shown: Binding<Bool>?

    func body(content: Content) -> some View {
        if let voice {
            @Bindable var voice = voice
            content.alert(MacVoiceCopy.confirmTitle, isPresented: shown ?? $voice.showsDownloadConfirm) {
                Button("ダウンロード") { voice.approveDownload() }
                Button("あとで", role: .cancel) { voice.declineDownload() }
            } message: {
                Text(MacVoiceCopy.confirmMessage(
                    advice: ModelDownloadAdvice.evaluate(megabytes: MacVoiceInput.modelMegabytes,
                                                         isExpensiveNetwork: ExpensiveNetworkProbe.shared.isExpensive,
                                                         availableBytes: ModelDownloadAdvice.availableBytesOnDevice()),
                    slowHardware: voice.isSlowHardware))
            }
        } else {
            content
        }
    }
}
