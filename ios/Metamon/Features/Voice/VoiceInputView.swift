import SwiftUI
import MetamonDSL
import MetamonServices

/// 音声入力画面。初期状態のアプリ全体でもあり、生成後は FAB からシートとしても開く。
/// 流れ: 録音 → WhisperKit(オンデバイス)文字起こし → ai サービス → 新 DSL をアニメーション付きで適用。
struct VoiceInputView: View {
    @Environment(AppRuntime.self) private var rt
    @Environment(\.services) private var services
    var prompt: String?

    @State private var phase: Phase = .idle
    @State private var transcript = ""
    @State private var typed = ""

    enum Phase { case idle, listening, transcribing, thinking }

    var body: some View {
        VStack(spacing: 28) {
            Spacer()
            Text(prompt ?? "どんなアプリが欲しいですか？")
                .font(.title2.bold())
                .multilineTextAlignment(.center)
            Text(statusText)
                .foregroundStyle(.secondary)
                .multilineTextAlignment(.center)
                .contentTransition(.opacity)

            #if targetEnvironment(simulator)
            // シミュレータでは WhisperKit(CoreML/ANE)が動かないため、マイクは出さず文字入力のみ。
            Label("シミュレータでは音声入力は使えません(文字で入力)", systemImage: "mic.slash")
                .font(.footnote)
                .foregroundStyle(.secondary)
            #else
            micButton
            modelStatusView
            #endif

            if !transcript.isEmpty {
                Text(transcript)
                    .padding()
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .background(.thinMaterial, in: RoundedRectangle(cornerRadius: 16))
                    .transition(.move(edge: .bottom).combined(with: .opacity))
            }
            Spacer()

            HStack {
                TextField("または文字で入力", text: $typed)
                    .textFieldStyle(.roundedBorder)
                    .submitLabel(.send)
                    .onSubmit { submitTyped() }
                Button("送信", action: submitTyped)
                    .buttonStyle(.borderedProminent)
                    .disabled(typed.isEmpty || rt.isMorphing)
            }
        }
        .padding(24)
        .animation(.spring(duration: 0.4), value: phase)
        .animation(.spring(duration: 0.4), value: transcript)
    }

    private var micButton: some View {
        Button { Task { await toggle() } } label: {
            ZStack {
                if phase == .listening {
                    Circle().stroke(.tint.opacity(0.3), lineWidth: 2)
                        .frame(width: 150, height: 150)
                        .scaleEffect(pulse ? 1.25 : 1)
                        .opacity(pulse ? 0 : 1)
                        .animation(.easeOut(duration: 1.2).repeatForever(autoreverses: false), value: pulse)
                }
                Circle()
                    .fill(phase == .listening ? Color.red : Color.accentColor)
                    .frame(width: 110, height: 110)
                    .shadow(color: .black.opacity(0.15), radius: 12, y: 6)
                Image(systemName: iconName)
                    .font(.system(size: 42, weight: .semibold))
                    .foregroundStyle(.white)
                    .contentTransition(.symbolEffect(.replace))
            }
        }
        .buttonStyle(.plain)
        .disabled(phase == .transcribing || phase == .thinking || (phase == .idle && !transcriber.isReady))
        .onAppear { pulse = true }
    }

    @State private var pulse = false

    private var transcriber: any SpeechTranscriber { services.transcriber }

    /// WhisperKit モデルの DL / ロード状況。ready になったら消える。
    @ViewBuilder
    private var modelStatusView: some View {
        switch transcriber.status {
        case .notDownloaded, .downloading:
            VStack(spacing: 8) {
                ProgressView(value: transcriber.downloadProgress)
                    .progressViewStyle(.linear)
                Text("音声モデルをダウンロード中 \(Int(transcriber.downloadProgress * 100))%\n初回のみ約500MBをダウンロードします(Wi-Fi 推奨)")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
                    .multilineTextAlignment(.center)
            }
            .padding(.horizontal)
        case .loading:
            HStack(spacing: 8) {
                ProgressView()
                Text("音声モデルを読み込み中…(初回は1分ほどかかります)")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }
        case .failed(let message):
            VStack(spacing: 8) {
                Text("音声モデルの準備に失敗しました\n\(message)")
                    .font(.footnote)
                    .foregroundStyle(.red)
                    .multilineTextAlignment(.center)
                    .lineLimit(3)
                Button("再試行") {
                    Task { try? await transcriber.prepare() }
                }
                .buttonStyle(.bordered)
            }
            .padding(.horizontal)
        case .ready, .recognizing:
            EmptyView()
        }
    }

    private var iconName: String {
        switch phase {
        case .idle: return "mic.fill"
        case .listening: return "stop.fill"
        case .transcribing: return "waveform"
        case .thinking: return "sparkles"
        }
    }

    private var statusText: String {
        switch phase {
        case .idle:
            #if targetEnvironment(simulator)
            return "下の欄に文字で入力してください"
            #else
            return transcriber.isReady ? "タップして話す" : "音声モデルを準備中…"
            #endif
        case .listening: return "聞いています… もう一度タップで終了"
        case .transcribing: return "文字起こし中…"
        case .thinking: return "アプリを考えています…"
        }
    }

    private func toggle() async {
        switch phase {
        case .idle:
            do {
                try await transcriber.startRecording()
                phase = .listening
            } catch {
                rt.toast = error.localizedDescription
            }
        case .listening:
            phase = .transcribing
            let text: String
            do {
                text = try await transcriber.stopAndTranscribe()
            } catch {
                rt.toast = "文字起こしに失敗しました: \(error.localizedDescription)"
                phase = .idle
                return
            }
            guard !text.isEmpty else {
                rt.toast = "音声を聞き取れませんでした。もう一度話してください"
                phase = .idle
                return
            }
            transcript = text
            phase = .thinking
            await rt.morph(transcript: text)
            phase = .idle
        default:
            break
        }
    }

    private func submitTyped() {
        let text = typed
        typed = ""
        transcript = text
        Task {
            phase = .thinking
            await rt.morph(transcript: text)
            phase = .idle
        }
    }
}

#Preview {
    let services = BuildConfig.make(environment: .mock)
    VoiceInputView()
        .environment(AppRuntime(document: .fallback, services: services))
        .environment(\.services, services)
}
