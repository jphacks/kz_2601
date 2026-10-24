import SwiftUI
import MetamonDSL
import MetamonServices

/// 音声入力画面。初期状態のアプリ全体でもあり、生成後は FAB からシートとしても開く。
/// 流れ: 録音 → WhisperKit(オンデバイス)文字起こし → ai サービス → 新 DSL をアニメーション付きで適用。
/// 生成中の演出は RootView(`GeneratingView` / `EditingIndicator`)が担うので、ここは「聞く」までに集中する。
struct VoiceInputView: View {
    @Environment(AppRuntime.self) private var rt
    @Environment(AuthSession.self) private var session
    @Environment(\.services) private var services
    @Environment(\.colorScheme) private var scheme
    var prompt: String?

    @State private var phase: Phase = .idle
    @State private var transcript = ""
    @State private var typed = ""
    @State private var showSettings = false
    @FocusState private var typing: Bool

    enum Phase { case idle, listening, transcribing, thinking }

    /// 初期状態(全画面)か、生成後のシート(改変)か。
    private var isInitial: Bool { rt.dsl.isVoiceOnly }

    /// 見出し・サブ文言(#42)。初回/再訪/新規/戻る/改変 × 時間帯。`prompt`(DSL の voiceInput.prompt)があれば見出しだけ上書き。
    private var greeting: Greeting.Copy {
        Greeting.copy(for: Greeting.context(homeEntry: rt.homeEntry, isInitial: isInitial))
    }

    var body: some View {
        ZStack {
            if isInitial { Theme.backdrop(scheme) }

            VStack(spacing: 0) {
                topBar
                Spacer(minLength: Theme.Spacing.md)
                heading
                Spacer(minLength: Theme.Spacing.lg)
                micSection
                Spacer(minLength: Theme.Spacing.lg)
                errorCard
                transcriptCard
                Spacer(minLength: Theme.Spacing.md)
                examples
                textEntry
                if isInitial { DiscoverEntry() } // 「探す」(#50)
            }
            .padding(.horizontal, Theme.Spacing.lg)
            .padding(.bottom, Theme.Spacing.md)
            .padding(.top, isInitial ? 0 : Theme.Spacing.md)
        }
        .animation(.spring(duration: 0.45, bounce: 0.15), value: phase)
        .animation(.spring(duration: 0.45, bounce: 0.15), value: transcript)
        .animation(.spring(duration: 0.45, bounce: 0.15), value: rt.errorCard)
        .sheet(isPresented: $showSettings) { SettingsSheet() }
        .onTapGesture { typing = false }
    }

    // MARK: - Sections

    private var topBar: some View {
        HStack {
            if isInitial {
                HStack(spacing: Theme.Spacing.sm) {
                    BrandMark(size: 28)
                    Text("Tailor It").font(.headline.weight(.bold))
                }
            }
            Spacer()
            Button { showSettings = true } label: {
                Image(systemName: "gearshape")
                    .font(.body.weight(.medium))
                    .foregroundStyle(.secondary)
                    .padding(Theme.Spacing.sm)
                    .background(Theme.Colors.surface, in: Circle())
                    .overlay(Circle().strokeBorder(Theme.Colors.hairline))
            }
            .accessibilityLabel("設定")
        }
        .padding(.top, Theme.Spacing.sm)
    }

    private var heading: some View {
        VStack(spacing: Theme.Spacing.sm) {
            Text(prompt ?? greeting.title)
                .font(.system(size: 30, weight: .bold, design: .rounded))
                .multilineTextAlignment(.center)
                .minimumScaleFactor(0.8)
                .contentTransition(.opacity)
            Text(greeting.subtitle)
                .font(.subheadline)
                .foregroundStyle(.secondary)
                .multilineTextAlignment(.center)
                .fixedSize(horizontal: false, vertical: true)
        }
        .animation(.easeInOut(duration: 0.3), value: greeting)
    }

    private var micSection: some View {
        VStack(spacing: Theme.Spacing.md) {
            // タップ = 発話、長押し = メタ操作(変身一覧)。FAB と同じ二段構え
            MicButton(mode: micMode, size: isInitial ? 132 : 104) { Task { await toggle() } }
                .simultaneousGesture(
                    LongPressGesture(minimumDuration: Theme.Layout.fabLongPressSeconds)
                        .onEnded { _ in if phase == .idle { rt.openSwitcher() } }
                )
                .accessibilityHint("長押しで変身一覧")
            Text(statusText)
                .font(.subheadline.weight(.medium))
                .foregroundStyle(phase == .listening ? Theme.Colors.recordA : .secondary)
                .contentTransition(.opacity)
                .id(statusText)
                .transition(.opacity)
            modelStatusView
            #if targetEnvironment(simulator)
            Text("シミュレータ: マイクはダミー(定型文を返します)")
                .font(.caption2).foregroundStyle(.tertiary)
            #endif
        }
    }

    /// エラーの状態カード(#60)。圏外 / 混雑 / タイムアウト / セッション切れを 1 文 + 主ボタンで。トーストは使わない。
    @ViewBuilder
    private var errorCard: some View {
        if let card = rt.errorCard {
            ErrorCard(presentation: card)
                .padding(.bottom, Theme.Spacing.md)
                .transition(.move(edge: .bottom).combined(with: .opacity))
        }
    }

    @ViewBuilder
    private var transcriptCard: some View {
        if !transcript.isEmpty, rt.errorCard?.kind != .offlineQueued {
            HStack(alignment: .top, spacing: Theme.Spacing.sm) {
                Image(systemName: "quote.opening")
                    .font(.caption.weight(.bold))
                    .foregroundStyle(Theme.brandGradient)
                    .padding(.top, 3)
                TypewriterText(text: transcript)
                    .font(.body.weight(.medium))
                    .frame(maxWidth: .infinity, alignment: .leading)
            }
            .metamonCard()
            .transition(.move(edge: .bottom).combined(with: .opacity))
        }
    }

    @ViewBuilder
    private var examples: some View {
        // 状態カードが出ている間は例文を引っ込める(縦に詰まって見出しが潰れるのを防ぎ、「次の一手」をカードに絞る)
        if phase == .idle, rt.errorCard == nil {
            VStack(alignment: .leading, spacing: Theme.Spacing.sm) {
                Text(isInitial ? Greeting.examplesLabel : Greeting.editExamplesLabel)
                    .font(.caption.weight(.semibold))
                    .foregroundStyle(.tertiary)
                ScrollView(.horizontal, showsIndicators: false) {
                    HStack(spacing: Theme.Spacing.sm) {
                        ForEach(isInitial ? Greeting.createExamples : Greeting.editExamples, id: \.self) { ex in
                            Button { submit(ex) } label: {
                                Text(ex)
                                    .font(.subheadline)
                                    .padding(.horizontal, Theme.Spacing.md)
                                    .padding(.vertical, Theme.Spacing.sm + 2)
                                    .background(Theme.Colors.surface, in: Capsule())
                                    .overlay(Capsule().strokeBorder(Theme.Colors.hairline))
                            }
                            .buttonStyle(.plain)
                            .disabled(rt.isMorphing)
                        }
                    }
                    .padding(.horizontal, 1)
                }
            }
            .padding(.bottom, Theme.Spacing.md)
            .transition(.opacity)
        }
    }

    private var textEntry: some View {
        HStack(spacing: Theme.Spacing.sm) {
            TextField("または文字で入力", text: $typed)
                .focused($typing)
                .submitLabel(.send)
                .onSubmit { submitTyped() }
                .padding(.horizontal, Theme.Spacing.md)
                .frame(height: 44)
                .background(Theme.Colors.surface, in: Capsule())
                .overlay(Capsule().strokeBorder(Theme.Colors.hairline))
            Button(action: submitTyped) {
                Image(systemName: "arrow.up")
                    .font(.body.weight(.bold))
                    .foregroundStyle(.white)
                    .frame(width: 44, height: 44)
                    .background(typed.isEmpty ? AnyShapeStyle(Color.gray.opacity(0.4)) : AnyShapeStyle(Theme.brandGradient), in: Circle())
            }
            .disabled(typed.isEmpty || rt.isMorphing)
            .accessibilityLabel("送信")
        }
    }

    // MARK: - State

    private var transcriber: any SpeechTranscriber { services.transcriber }

    private var micMode: MicButton.Mode {
        switch phase {
        case .idle: .idle(ready: transcriber.isReady)
        case .listening: .listening
        case .transcribing, .thinking: .transcribing
        }
    }

    /// WhisperKit モデルの DL / ロード状況。ready になったら消える。
    @ViewBuilder
    private var modelStatusView: some View {
        switch transcriber.status {
        case .notDownloaded, .downloading:
            VStack(spacing: Theme.Spacing.sm) {
                ProgressView(value: transcriber.downloadProgress)
                    .progressViewStyle(.linear)
                    .tint(Theme.Colors.brandA)
                Text("音声モデルをダウンロード中 \(Int(transcriber.downloadProgress * 100))%\n初回のみ約500MB(Wi-Fi 推奨)")
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .multilineTextAlignment(.center)
            }
            .padding(.horizontal, Theme.Spacing.lg)
        case .loading:
            HStack(spacing: Theme.Spacing.sm) {
                ProgressView().controlSize(.small)
                Text("音声モデルを読み込み中…(初回は1分ほど)")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
        case .failed(let message):
            VStack(spacing: Theme.Spacing.sm) {
                Text("音声モデルの準備に失敗しました\n\(message)")
                    .font(.caption)
                    .foregroundStyle(.red)
                    .multilineTextAlignment(.center)
                    .lineLimit(3)
                Button("再試行") { Task { try? await transcriber.prepare() } }
                    .buttonStyle(.bordered)
                    .controlSize(.small)
            }
            .padding(.horizontal, Theme.Spacing.lg)
        case .ready, .recognizing:
            EmptyView()
        }
    }

    private var statusText: String {
        switch phase {
        case .idle: return transcriber.isReady ? "タップして話す" : "音声モデルを準備中…"
        case .listening: return "聞いています… もう一度タップで終了"
        case .transcribing: return "文字起こし中…"
        case .thinking: return "アプリを考えています…"
        }
    }

    // MARK: - Actions

    private func toggle() async {
        switch phase {
        case .idle:
            typing = false
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
            } catch let failure as TranscriberFailure {
                // 短すぎ / 無音 / マイク未入力 / セッション失敗。理由はそのまま日本語で出る
                rt.toast = failure.localizedDescription
                phase = .idle
                return
            } catch {
                rt.toast = "文字起こしに失敗しました: \(error.localizedDescription)"
                phase = .idle
                return
            }
            guard !text.isEmpty else {
                let detail = transcriber.lastDiagnostics.map { "(\($0.summary))" } ?? ""
                rt.toast = "音声を聞き取れませんでした\(detail)。もう一度はっきり話してください"
                phase = .idle
                return
            }
            transcript = text
            // 1 文字ずつ出すのを見せてから送る(edit はシートを閉じて本体側のインジケータに任せる)
            try? await Task.sleep(for: .milliseconds(min(900, 40 * text.count)))
            await morph(text)
        default:
            break
        }
    }

    private func submitTyped() {
        let text = typed.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !text.isEmpty else { return }
        typed = ""
        typing = false
        submit(text)
    }

    private func submit(_ text: String) {
        transcript = text
        Task { await morph(text) }
    }

    private func morph(_ text: String) async {
        phase = .thinking
        if !isInitial { rt.showVoiceSheet = false }
        await rt.morph(transcript: text)
        phase = .idle
    }
}

#Preview {
    let services = BuildConfig.make(environment: .mock)
    VoiceInputView()
        .environment(AppRuntime(document: (try? DSLExamples.document("initial")) ?? .fallback, services: services))
        .environment(AuthSession(service: services.auth))
        .environment(\.services, services)
}
