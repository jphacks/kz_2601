import SwiftUI

/// 音声入力画面。初期状態のアプリ全体でもあり、生成後は FAB からシートとしても開く。
/// 流れ: 録音 → WhisperKit(オンデバイス)文字起こし → ai サービス → 新 DSL をアニメーション付きで適用。
struct VoiceInputView: View {
    @Environment(AppRuntime.self) private var rt
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
                .contentTransition(.opacity)

            micButton

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
        .disabled(phase == .transcribing || phase == .thinking)
        .onAppear { pulse = true }
    }

    @State private var pulse = false

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
        case .idle: return rt.services.transcriber.isReady ? "タップして話す" : "音声モデルを準備中…"
        case .listening: return "聞いています… もう一度タップで終了"
        case .transcribing: return "文字起こし中…"
        case .thinking: return "アプリを考えています…"
        }
    }

    private func toggle() async {
        switch phase {
        case .idle:
            do {
                try await rt.services.transcriber.startRecording()
                phase = .listening
            } catch {
                rt.toast = error.localizedDescription
            }
        case .listening:
            phase = .transcribing
            let text = (try? await rt.services.transcriber.stopAndTranscribe()) ?? ""
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
    VoiceInputView()
        .environment(AppRuntime(document: .fallback, services: .mock()))
}
