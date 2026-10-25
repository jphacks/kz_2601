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
    /// 対話中(初期画面の新規作成も、生成後のシートの改変も・#186)。
    private var inChat: Bool { rt.isChatting || (!isInitial && rt.hasChatHistory) } // #210 生成後のシートは履歴があれば続きを見せる

    /// 見出し・サブ文言(#42)。初回/再訪/新規/戻る/改変 × 時間帯。`prompt`(DSL の voiceInput.prompt)があれば見出しだけ上書き。
    private var greeting: Greeting.Copy {
        Greeting.copy(for: Greeting.context(homeEntry: rt.homeEntry, isInitial: isInitial))
    }

    var body: some View {
        ZStack {
            if isInitial { Theme.backdrop(scheme) }

            VStack(spacing: 0) {
                topBar
                if inChat {
                    // 対話(#75 / #186): 吹き出し + 答え / 質問チップ / 案のカード / 要約 +「これでいく」。
                    // 入力欄のキーボード追従は ClarifyChatView の safeAreaInset だけに任せる(#208):
                    // ここで safeAreaInset や phase のアニメーションを重ねると、返事が届いた瞬間(phase: thinking → idle)に
                    // キーボード分の安全領域ごとアニメーションし直して入力欄がずれうる
                    ClarifyChatView(micMode: micMode, statusText: statusText, onMic: { Task { await toggle() } },
                                    onSend: { submit($0) })
                        .transition(.opacity)
                } else {
                    idleContent
                        // 例文・文字入力・「探す」は下に固定し、キーボードと同じアニメーションで一緒に上げる(#196)
                        .safeAreaInset(edge: .bottom, spacing: 0) {
                            VStack(spacing: 0) {
                                if !typing { examples }
                                textEntry
                                if isInitial, !typing { DiscoverEntry() } // 「探す」(#50)
                            }
                            .padding(.bottom, Theme.Spacing.sm)
                        }
                        .animation(.spring(duration: 0.45, bounce: 0.15), value: phase)
                        .animation(.spring(duration: 0.45, bounce: 0.15), value: transcript)
                        .animation(.spring(duration: 0.45, bounce: 0.15), value: rt.errorCard)
                }
            }
            .padding(.horizontal, Theme.Spacing.lg)
            .padding(.top, isInitial ? 0 : Theme.Spacing.md)
        }
        .animation(.spring(duration: 0.45, bounce: 0.15), value: inChat)
        // typing には明示アニメーションを付けない: キーボードの持ち上げが別カーブで動いて入力欄が遅れて見えていた(#196)
        // 保険(#200): 初期画面から create が始まったら、この画面の上にも全画面の生成中表示を重ねる。
        // 本来は RootView がこの画面ごと GeneratingView に置き換えるが、CI(iOS 26.5)の声の自動生成では
        // RootView が morphJob の変化で作り直されず、対話画面のまま完成まで見えていた(この画面の中の部品は更新されていた)。
        // RootView が正しく置き換えればこの画面ごと消えるので、二重に残ることはない
        .overlay {
            if isInitial, let job = rt.morphJob, job.kind == .create {
                GeneratingView(job: job)
                    #if DEBUG
                    .onAppear { MorphProbe.fallbackAppeared += 1 }
                    #endif
            }
        }
        .sheet(isPresented: $showSettings) { SettingsSheet() }
        .onTapGesture { typing = false }
        // #43 初期画面の挨拶を読み上げ(対話中・生成中・生成後のシートでは読まない)
        .task(id: "\(isInitial)|\(inChat)|\(greeting.title)") {
            // 対話 →「この内容で作る」直後は inChat=false でも生成開始が数フレーム遅れるので、少し待ってから判定する
            try? await Task.sleep(for: .milliseconds(400))
            if !Task.isCancelled, isInitial, !inChat, !rt.isMorphing { SpeechReader.shared.speak((prompt ?? greeting.title) + "\n" + greeting.subtitle) }
            #if DEBUG
            // ヘッドレス E2E 用: `METAMON_AUTO_MIC=<秒>` で読み上げ中にマイクをタップ(読み上げ停止 → 録音の確認)
            if let s = ProcessInfo.processInfo.environment["METAMON_AUTO_MIC"].flatMap(Double.init), isInitial, phase == .idle {
                try? await Task.sleep(for: .seconds(s))
                if !Task.isCancelled { await toggle() }
            }
            #endif
        }
        #if DEBUG
        .task {
            // ヘッドレス E2E 用: `METAMON_AUTO_SETTINGS=1` で起動 2 秒後に設定シートを開く(言語切替の確認・#44)
            guard ProcessInfo.processInfo.environment["METAMON_AUTO_SETTINGS"] == "1" else { return }
            try? await Task.sleep(for: .seconds(2))
            showSettings = true
        }
        .task {
            // ヘッドレス E2E 用: `METAMON_AUTO_FOCUS=voice` で 3 秒後に文字入力へフォーカス(キーボードの確認・#143)
            let key = ProcessInfo.processInfo.environment["METAMON_AUTO_FOCUS"]
            guard key == "voice" || key == "voice-sheet", !inChat else { return }
            try? await Task.sleep(for: .seconds(3))
            typing = true
        }
        #endif
    }

    // MARK: - Sections

    /// 対話の前(マイク・見出し・書き起こし)。
    private var idleContent: some View {
        VStack(spacing: 0) {
            Spacer(minLength: Theme.Spacing.md)
            ScreenContextChip() // 画面マップで選んだ画面(#116)
            heading
            Spacer(minLength: Theme.Spacing.lg)
            // 文字入力中はマイク・例文・「探す」を畳み、入力欄をキーボードの直上に置く(縦に収まらず入力欄が隠れていた・#143)
            if !typing {
                micSection
                Spacer(minLength: Theme.Spacing.lg)
            }
            errorCard
            transcriptCard
            Spacer(minLength: Theme.Spacing.md)
        }
    }

    private var topBar: some View {
        HStack {
            if isInitial {
                HStack(spacing: Theme.Spacing.sm) {
                    BrandMark(size: 28)
                    Text("Tailor It").font(.headline.weight(.bold))
                }
            } else {
                UndoHeaderButtons() // #209 修正チャットの上部に「1つ前に戻す」
            }
            Spacer()
            if inChat {
                Button { rt.restartChat() } label: {
                    Label(ClarifyCopy.restart, systemImage: "arrow.counterclockwise")
                        .font(.subheadline.weight(.medium))
                        .foregroundStyle(.secondary)
                        .padding(.horizontal, Theme.Spacing.md)
                        .padding(.vertical, Theme.Spacing.sm)
                        .background(Theme.Colors.surface, in: Capsule())
                        .overlay(Capsule().strokeBorder(Theme.Colors.hairline))
                }
                .disabled(rt.isMorphing)
                .accessibilityIdentifier("chat.restart")
            }
            Button { showSettings = true } label: {
                Image(systemName: "gearshape")
                    .font(.body.weight(.medium))
                    .foregroundStyle(.secondary)
                    .padding(Theme.Spacing.sm)
                    .background(Theme.Colors.surface, in: Circle())
                    .overlay(Circle().strokeBorder(Theme.Colors.hairline))
            }
            .accessibilityLabel("設定")
            .accessibilityIdentifier("voice.settings")
        }
        .padding(.top, Theme.Spacing.sm)
    }

    private var heading: some View {
        VStack(spacing: Theme.Spacing.sm) {
            if isInitial, prompt == nil { CraneMascot(pose: .happy, size: 76) } // #102
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

    /// 文字入力(#196): 伸びる複数行・return は改行・送信はボタン。送信すると対話か生成に進むのでキーボードは閉じる。
    private var textEntry: some View {
        ChatComposer(placeholder: L10n.tr("または文字で入力"), text: $typed, focused: $typing,
                     disabled: rt.isMorphing, keepsFocus: false) { submit($0) }
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
        case .idle: return transcriber.isReady ? L10n.tr("タップして話す") : L10n.tr("音声モデルを準備中…")
        case .listening: return L10n.tr("聞いています… もう一度タップで終了")
        case .transcribing: return L10n.tr("文字起こし中…")
        case .thinking: return rt.isChatting ? ClarifyCopy.thinking : L10n.tr("アプリを考えています…")
        }
    }

    // MARK: - Actions

    private func toggle() async {
        switch phase {
        case .idle:
            typing = false
            await SpeechReader.shared.stopForRecording() // #43 読み上げ中なら即停止してから録音
            do {
                try await transcriber.startRecording()
                phase = .listening
            } catch {
                SpeechReader.shared.recordingFinished()
                rt.toast = ErrorCopy.message(for: error)
            }
        case .listening:
            phase = .transcribing
            let stoppedAt = ContinuousClock.now // #160 区間ログ
            SpeechReader.shared.recordingFinished()
            let text: String
            do {
                text = try await transcriber.stopAndTranscribe()
            } catch let failure as TranscriberFailure {
                // 短すぎ / 無音 / マイク未入力 / セッション失敗。理由はそのまま日本語で出る
                rt.toast = ErrorCopy.transcriberFailure(failure)
                phase = .idle
                return
            } catch {
                rt.toast = L10n.tr("文字起こしに失敗しました: \(error.localizedDescription)")
                phase = .idle
                return
            }
            guard !text.isEmpty else {
                let detail = transcriber.lastDiagnostics.map { "(\(ErrorCopy.diagnosticsSummary($0)))" } ?? ""
                rt.toast = L10n.tr("音声を聞き取れませんでした\(detail)。もう一度はっきり話してください")
                phase = .idle
                return
            }
            if rt.routesToChat(text) {
                // 対話: 書き起こしはそのまま吹き出しになるので、タイプライター演出は挟まない
                await morph(text, voice: ChatTiming(recordStopped: stoppedAt, transcribed: .now))
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

    private func submit(_ text: String) {
        if !rt.routesToChat(text) { transcript = text }
        Task { await morph(text) }
    }

    /// 対話 ON なら create も edit も `submitUtterance`(チャットへ・#186)。
    /// 対話 OFF・「元に戻して」等の操作は従来どおり: 初期画面は即生成、生成後のシートは閉じて即反映。
    private func morph(_ text: String, voice: ChatTiming? = nil) async {
        phase = .thinking
        SpeechReader.shared.stop()
        if isInitial || rt.routesToChat(text) {
            await rt.submitUtterance(text, source: voice == nil ? .text : .voice, timing: voice ?? ChatTiming())
        } else {
            rt.showVoiceSheet = false
            await rt.morph(transcript: text, about: ScreenMapUI.shared.takeContext())
        }
        phase = .idle
        if rt.isChatting { transcript = "" }
    }
}

#Preview {
    let services = BuildConfig.make(environment: .mock)
    VoiceInputView()
        .environment(AppRuntime(document: (try? DSLExamples.document("initial")) ?? .fallback, services: services))
        .environment(AuthSession(service: services.auth))
        .environment(\.services, services)
}
