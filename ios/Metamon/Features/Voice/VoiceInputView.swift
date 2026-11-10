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
    /// 横向き(#625)はマイク・見出しを左、例文・文字入力を右に並べる
    @Environment(\.verticalSizeClass) private var verticalSizeClass
    var prompt: String?

    @State private var phase: Phase = .idle
    @State private var transcript = ""
    @State private var typed = ""
    @State private var showSettings = false
    /// 頼み方のコツ(#415)
    @State private var showTips = false
    /// 初期画面の例文のカテゴリ(#274)
    @State private var exampleCategory = "record"
    @FocusState private var typing: Bool
    #if !APP_STORE_REVIEW
    /// 続けて失敗している自動の動きの「直しますか?」(#347)。生成後のシートで、作成者の時だけ中身が入る
    @State private var automationFix: AutomationFixOfferModel?
    #endif

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
                    #if APP_STORE_REVIEW
                    ClarifyChatView(micMode: micMode, statusText: statusText, onMic: { Task { await toggle() } },
                                    onSend: { submit($0) })
                        .readableWidth() // 横向きで吹き出し・入力欄が横に間延びしないように(#625)
                        .transition(.opacity)
                    #else
                    ClarifyChatView(micMode: micMode, statusText: statusText, onMic: { Task { await toggle() } },
                                    onSend: { submit($0) }, automationFix: automationFixModel)
                        .readableWidth() // 横向きで吹き出し・入力欄が横に間延びしないように(#625)
                        .transition(.opacity)
                    #endif
                } else if compactHeight {
                    // 横向き(#625): 縦に積むと入力欄の上に見出しとマイクが入らないので 2 列にする。
                    // 文字・録音の状態はこの View の @State なので、回転して並べ方が変わっても残る
                    HStack(alignment: .top, spacing: Theme.Spacing.lg) {
                        ScrollView { idleContent }
                            .scrollBounceBehavior(.basedOnSize)
                            .frame(maxWidth: .infinity)
                        VStack(spacing: 0) {
                            Spacer(minLength: Theme.Spacing.sm)
                            bottomPanel
                        }
                        .frame(maxWidth: .infinity)
                    }
                    .animation(.spring(duration: 0.45, bounce: 0.15), value: phase)
                    .animation(.spring(duration: 0.45, bounce: 0.15), value: rt.errorCard)
                } else {
                    idleContent
                        // 例文・文字入力・「探す」は下に固定し、キーボードと同じアニメーションで一緒に上げる(#196)
                        .safeAreaInset(edge: .bottom, spacing: 0) { bottomPanel }
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
        #if !APP_STORE_REVIEW
        .coinSheets() // #630 音声シートの上にお店を重ねる(コインが足りない 402・「約 N コイン」から)
        #endif
        .sheet(isPresented: $showSettings) { SettingsSheet() }
        .sheet(isPresented: $showTips) {
            PromptTipsSheet { text in
                showTips = false
                insertDraft(text)
            }
        }
        // 設定の「頼み方のコツ」で選んだ例(対話中は ClarifyChatView が受け取る)
        .onChange(of: rt.composerDraft, initial: true) { _, draft in
            guard let draft, !inChat else { return }
            rt.composerDraft = nil
            insertDraft(draft)
        }
        .onTapGesture { typing = false }
        #if !APP_STORE_REVIEW
        // 開いた時に 1 回だけ取りに行く(待たせない・取れなければ何も出さない)
        .task(id: rt.document.appId) {
            guard !isInitial, rt.dsl.serverRuleCount > 0, let client = services.automations else { automationFix = nil; return }
            let model = AutomationFixOfferModel(appId: rt.document.appId, client: client)
            await model.load()
            guard !Task.isCancelled else { return }
            withAnimation(.easeInOut(duration: 0.25)) { automationFix = model }
        }
        #endif
        // 録音中にシートを下に払って閉じたら録音を捨てる(マイクが点いたまま残り、次に話した時に前の音が混ざっていた)
        .onDisappear {
            guard phase == .listening else { return }
            transcriber.cancelRecording()
            SpeechReader.shared.recordingFinished()
            phase = .idle
        }
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

    private var compactHeight: Bool { LandscapeLayout.isCompactHeight(verticalSizeClass) }

    /// 例文・文字入力・「探す」。縦向きは画面の下に固定、横向きは右の列の下に置く。
    private var bottomPanel: some View {
        VStack(spacing: 0) {
            if !typing {
                #if !APP_STORE_REVIEW
                if let automationFixModel, phase == .idle, rt.errorCard == nil {
                    AutomationFixOfferRow(model: automationFixModel).padding(.bottom, Theme.Spacing.sm)
                }
                #endif
                examples
            }
            RemainingTodayBadge() // #630 今日の残り
            #if !APP_STORE_REVIEW
            CoinEstimateLine(operation: rt.dsl.isVoiceOnly ? .generate : .edit) // #630 送る前の「約 N コイン」
            #endif
            textEntry
            #if !APP_STORE_REVIEW
            if isInitial, !typing { DiscoverEntry() } // 「探す」(#50)
            #endif
        }
        .padding(.bottom, Theme.Spacing.sm)
    }

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
                UndoHeaderButtons() // #209 修正チャットの上部に「元に戻す」
            }
            Spacer()
            if inChat {
                Button { rt.requestRestartChat() } label: {
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
                .modifier(RestartChatConfirm())
            }
            // H10: 一覧はマイクの長押しにしか無かった。設定の隣に見える入口を置く(作ったアプリが無い時は出さない)
            if VoiceTopBar.showsAppList(appCount: rt.visibleApps.count, isVoiceOnly: rt.dsl.isVoiceOnly, madeFirstApp: rt.hasMadeFirstApp) {
                Button { rt.openSwitcher() } label: {
                    Image(systemName: "square.grid.2x2")
                        .font(.body.weight(.medium))
                        .foregroundStyle(.secondary)
                        .padding(Theme.Spacing.sm)
                        .background(Theme.Colors.surface, in: Circle())
                        .overlay(Circle().strokeBorder(Theme.Colors.hairline))
                }
                .disabled(rt.isMorphing)
                .accessibilityLabel(Text(L10n.key("変身一覧", review: "記録帳の一覧")))
                .accessibilityIdentifier("voice.appList")
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
            // 横向きは縦の空きが無いのでツルを出さない(#625)
            if isInitial, prompt == nil, !compactHeight { CraneMascot(pose: .happy, size: 76) } // #102
            Text(prompt ?? greeting.title)
                .font(.system(size: LandscapeLayout.headingSize(compactHeight: compactHeight), weight: .bold, design: .rounded))
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
            MicButton(mode: micMode, size: LandscapeLayout.micSize(isInitial: isInitial, compactHeight: compactHeight)) {
                if phase == .idle, rt.askModelDownloadBeforeRecording() { return } // M14: 起動では聞かず、初めてマイクを押した時に
                Task { await toggle() }
            }
                .simultaneousGesture(
                    LongPressGesture(minimumDuration: Theme.Layout.fabLongPressSeconds)
                        .onEnded { _ in if phase == .idle { rt.openSwitcher() } }
                )
                .accessibilityHint(L10n.key("長押しで変身一覧", review: "長押しで記録帳の一覧"))
            Text(statusText)
                .font(.subheadline.weight(.medium))
                .foregroundStyle(phase == .listening ? Theme.Colors.recordA : .secondary)
                .contentTransition(.opacity)
                .id(statusText)
                .transition(.opacity)
            if transcriber.isProvisional {
                // U5: WhisperKit の準備中は Apple の端末内の認識で仮に聞き取る。精度が違うことを小さく伝える
                Label(VoiceModelCopy.provisional, systemImage: "waveform.badge.exclamationmark")
                    .font(.caption2)
                    .foregroundStyle(.secondary)
                    .accessibilityIdentifier("voice.provisional")
            }
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
                HStack {
                    Text(isInitial ? Greeting.examplesLabel : Greeting.editExamplesLabel)
                        .font(.caption.weight(.semibold))
                        .foregroundStyle(.tertiary)
                    Spacer()
                    PromptTipsLink { showTips = true }
                }
                if isInitial { exampleCategoryPicker }
                ScrollView(.horizontal, showsIndicators: false) {
                    HStack(spacing: Theme.Spacing.sm) {
                        ForEach(exampleChips, id: \.self) { ex in
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
                .id(exampleCategory) // カテゴリを替えたら先頭から見せる
            }
            .padding(.bottom, Theme.Spacing.md)
            .transition(.opacity)
        }
    }

    private var exampleCategories: [Greeting.ExampleCategory] { Greeting.createExampleCategories() }

    private var exampleChips: [String] {
        guard isInitial else { return Greeting.editExamples }
        return exampleCategories.first { $0.id == exampleCategory }?.examples ?? Greeting.createExamples
    }

    /// 例文のカテゴリ(記録・習慣・お金・家族/友達と・ゲーム・#274)。選んだカテゴリの例が下の行に並ぶ。
    private var exampleCategoryPicker: some View {
        ScrollView(.horizontal, showsIndicators: false) {
            HStack(spacing: Theme.Spacing.xs) {
                ForEach(exampleCategories) { category in
                    let selected = category.id == exampleCategory
                    Button {
                        withAnimation(.easeInOut(duration: 0.2)) { exampleCategory = category.id }
                    } label: {
                        Text(category.title)
                            .font(.caption.weight(.semibold))
                            .foregroundStyle(selected ? Color.white : Color.secondary)
                            .padding(.horizontal, Theme.Spacing.sm + 4)
                            .padding(.vertical, Theme.Spacing.xs + 2)
                            .background {
                                if selected { Capsule().fill(Theme.brandGradient) }
                            }
                            .contentShape(Capsule())
                    }
                    .buttonStyle(.plain)
                    .accessibilityAddTraits(selected ? .isSelected : [])
                    .accessibilityIdentifier("examples.category.\(category.id)")
                }
            }
            .padding(.horizontal, 1)
        }
    }

    /// 文字入力(#196): 伸びる複数行・return は改行・送信はボタン。送信すると対話か生成に進むのでキーボードは閉じる。
    private var textEntry: some View {
        // 音声の準備ができるまでは、文字ですぐ作れることを入力欄でも伝える(#275)
        ChatComposer(placeholder: transcriber.isReady ? L10n.tr("または文字で入力") : VoiceModelCopy.textPlaceholder,
                     text: $typed, focused: $typing,
                     disabled: rt.isMorphing, keepsFocus: false) { submit($0) }
    }

    // MARK: - State

    #if !APP_STORE_REVIEW
    private var automationFixModel: AutomationFixOfferModel? { isInitial ? nil : automationFix }
    #endif

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
        case .notDownloaded where !ModelDownloadConsent.isApproved() || ExpensiveNetworkProbe.shared.isExpensive:
            // 確認で「あとで」を選んだ / モバイル回線で待っている(#275): 文字で始められることを先に伝える
            VStack(spacing: Theme.Spacing.sm) {
                Text(transcriber.isProvisional ? VoiceModelCopy.provisionalTextFirst : VoiceModelCopy.textFirst)
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .multilineTextAlignment(.center)
                    .fixedSize(horizontal: false, vertical: true)
                Button(VoiceModelCopy.downloadButton) { rt.showModelDownloadConfirm = true }
                    .buttonStyle(.bordered)
                    .controlSize(.small)
                    .accessibilityIdentifier("voice.modelDownload")
            }
            .padding(.horizontal, Theme.Spacing.lg)
        case .notDownloaded, .downloading:
            VStack(spacing: Theme.Spacing.sm) {
                ProgressView(value: transcriber.downloadProgress)
                    .progressViewStyle(.linear)
                    .tint(Theme.Colors.brandA)
                Text(VoiceModelCopy.downloading(percent: Int(transcriber.downloadProgress * 100)))
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .multilineTextAlignment(.center)
                    .fixedSize(horizontal: false, vertical: true)
            }
            .padding(.horizontal, Theme.Spacing.lg)
        case .loading:
            HStack(spacing: Theme.Spacing.sm) {
                ProgressView().controlSize(.small)
                Text("音声モデルを読み込み中…(初回は1分ほど)")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
        case .failed: // 中身はログ(`[Voice]`)と設定の詳細にだけ出す(H9)
            VStack(spacing: Theme.Spacing.sm) {
                Text("音声モデルを用意できませんでした。接続を確認して「再試行」を押してください")
                    .font(.caption)
                    .foregroundStyle(.red)
                    .multilineTextAlignment(.center)
                    .lineLimit(3)
                Text(VoiceModelCopy.textStillWorks)
                    .font(.caption)
                    .foregroundStyle(.secondary)
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
        case .idle: return transcriber.isReady ? L10n.tr("タップして話す") : VoiceModelCopy.notReadyStatus
        case .listening: return L10n.tr("聞いています… もう一度タップで終了")
        case .transcribing: return L10n.tr("文字起こし中…")
        case .thinking: return rt.isChatting ? ClarifyCopy.thinking : L10n.pick(L10n.tr("アプリを考えています…"), review: L10n.tr("記録帳の形を考えています…"))
        }
    }

    // MARK: - Actions

    private func toggle() async {
        switch phase {
        case .idle:
            typing = false
            await SpeechReader.shared.stopForRecording() // #43 読み上げ中なら即停止してから録音
            if case .notice = rt.errorCard?.kind { withAnimation { rt.errorCard = nil } } // 前の録音の失敗は話し直したら消す
            do {
                try await transcriber.startRecording()
                phase = .listening
            } catch {
                SpeechReader.shared.recordingFinished()
                showVoiceFailure(error)
            }
        case .listening:
            phase = .transcribing
            let stoppedAt = ContinuousClock.now // #160 区間ログ
            SpeechReader.shared.recordingFinished()
            let text: String
            do {
                text = try await transcriber.stopAndTranscribe()
            } catch {
                // 短すぎ / 無音 / マイク未入力 / セッション失敗など。消えない状態カードに次の一手を出す(H9)
                showVoiceFailure(error)
                phase = .idle
                return
            }
            guard !text.isEmpty else {
                showVoiceFailure(nil)
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

    /// 録音・文字起こしの失敗を状態カードに(H9・トーストだと 3 秒で消える)。診断の値はログにだけ残す。
    private func showVoiceFailure(_ error: Error?) {
        withAnimation(.easeInOut(duration: 0.3)) {
            rt.errorCard = .voiceFailure(error, diagnostics: transcriber.lastDiagnostics)
        }
    }

    /// 例文を入力欄に入れてキーボードを出す(送らない・#415)。シートが閉じきってからフォーカスする。
    private func insertDraft(_ text: String) {
        typed = text
        Task { @MainActor in
            try? await Task.sleep(for: .milliseconds(500))
            typing = true
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

/// 音声画面の上部(H10)。
enum VoiceTopBar {
    /// 一覧のボタンを出すか。今アプリを開いている・前にアプリを作った・一覧に 1 件でもある時。
    /// 初回(まだ何も作っていない初期画面)は空の一覧しか開けないので出さない。
    static func showsAppList(appCount: Int, isVoiceOnly: Bool, madeFirstApp: Bool) -> Bool {
        !isVoiceOnly || madeFirstApp || appCount > 0
    }
}

/// 音声モデルの準備中・未ダウンロードの文言(#275)。文字入力ならすぐ作れることを前に出す。
@MainActor
enum VoiceModelCopy {
    static var textFirst: String {
        L10n.tr("声で話すには、声を文字にするモデル(約\(AppRuntime.voiceModelMegabytes)MB)が必要です。\n下の欄に文字で入力すれば、今すぐ作れます。")
    }
    static var downloadButton: String { L10n.tr("音声モデルをダウンロード") }
    static func downloading(percent: Int) -> String {
        L10n.tr("音声モデルをダウンロード中 \(percent)%(初回のみ約\(AppRuntime.voiceModelMegabytes)MB)\n待たなくても、下の欄に文字で入力すれば作れます")
    }
    static var textStillWorks: String { L10n.tr("文字入力なら、このまま作れます") }
    static var notReadyStatus: String { L10n.tr("音声の準備中です(文字ならすぐ作れます)") }
    static var textPlaceholder: String { L10n.tr("文字で入力(今すぐ作れます)") }
    // U5: 本命のモデルの準備中に、端末内の Apple の音声認識で仮に聞き取っている時
    static var provisional: String { L10n.tr("準備中のため簡易の聞き取りです") }
    static var provisionalTextFirst: String {
        L10n.tr("今は簡易の聞き取りで話せます。音声モデル(約\(AppRuntime.voiceModelMegabytes)MB)を入れると、より正確に聞き取れます。")
    }
}

#Preview {
    let services = BuildConfig.make(environment: .mock)
    VoiceInputView()
        .environment(AppRuntime(document: (try? DSLExamples.document("initial")) ?? .fallback, services: services))
        .environment(AuthSession(service: services.auth))
        .environment(\.services, services)
}

/// 「最初から」の確認(M9)。これまでの会話(反映の区切り)も消すと戻せないので、今の会話だけ消すのと選べるようにする。
private struct RestartChatConfirm: ViewModifier {
    @Environment(AppRuntime.self) private var rt

    func body(content: Content) -> some View {
        @Bindable var rt = rt
        content.confirmationDialog(L10n.tr("最初からにしますか?"), isPresented: $rt.showRestartConfirm, titleVisibility: .visible) {
            if rt.isChatting {
                Button(L10n.tr("今の会話だけ消す")) { rt.restartLiveChat() }
                    .accessibilityIdentifier("chat.restart.live")
            }
            Button(L10n.tr("これまでの会話も消す"), role: .destructive) { rt.restartChat() }
                .accessibilityIdentifier("chat.restart.all")
            Button(L10n.tr("キャンセル"), role: .cancel) {}
        } message: {
            Text(L10n.tr("これまでの会話と反映の記録を消すと、元に戻せません。"))
        }
    }
}
