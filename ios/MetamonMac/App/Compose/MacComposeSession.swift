import SwiftUI
import AppKit
import Observation
import MetamonRenderer
import MetamonDSL
import MetamonServices

/// アプリを作る・直す会話(#967)の状態。iOS の `AppRuntime+Clarify` / `AppRuntime+BuildStage` の Mac 版。
/// 判断は iOS と同じ `ClarifyFlow`(何を出すか)・`MultiSelect`(複数選択)・`BuildChoreography`(組み上げ)を使う。
/// AI は `MacServices`(#930): Firebase につながれば ai サービス、つながらない時(plist 無し・CI・METAMON_MOCK=1)は `MockAIClient`(同梱の DSL 例を返す)。
/// AI に送る前に同意(#246・5.1.2(i))を確かめる。無ければ送らずに同意の説明を出し、同意したら止めた 1 件を送る(iOS の `requireAIConsent`)。
/// 流れ: 頼む(文字)→ 聞き返し・案(chat)→「これでいく」→ 生成中(組み立て台・経過秒)→ 組み上げ → 完成の要約。
@MainActor
@Observable
final class MacComposeSession: Identifiable {
    /// 何を相手に話しているか。edit は今中央で開いているアプリを直す
    enum Target: Equatable {
        case create
        case edit(appId: String, name: String, version: Int)
    }

    struct Bubble: Identifiable, Equatable, QuestionProgressBubble {
        let id = UUID()
        var role: ChatMessage.Role
        var text: String
        /// assistant の時だけ。応答の kind(「質問 n / 3」を数える・#1211)
        var kind: ChatTurn.Kind? = nil

        var isAssistantQuestion: Bool { role == .assistant && kind == .question }
    }

    /// 生成の 1 回分。経過秒・組み立て台の台本・SSE の仮置き(モックは届かない)
    struct Job: Equatable {
        var transcript: String
        var started: Date
        var script: BuildChoreography.WaitingScript
        var live = BuildChoreography.LiveDraft()
    }

    enum Phase: Equatable {
        /// 会話中(入力・聞き返し・案)
        case chatting
        /// 結果を待っている
        case generating(Job)
        /// 結果の部品を置いている(create だけ)
        case revealing(Job, BuildChoreography.AssemblyPlan)
        /// 完成。要約を出して閉じるのを待つ
        case finished(summary: String, unchanged: Bool)
    }

    let id = UUID()
    let target: Target
    private(set) var bubbles: [Bubble] = []
    /// このアプリの閉じた会話と反映の区切り(#1211・iOS の `chatHistory`)。今の会話(`bubbles`)は含まない。
    /// 端末には `history + 今の会話` を保存する(吹き出しが増えるたび)ので、シートを開き直すと続きから話せる
    private(set) var history: [ChatHistoryEntry] = []
    /// 「最初から」の確認(これまでの会話があるとき・iOS の M9)
    var showsRestartConfirm = false
    /// 最後の応答(チップ・案・要約カードはこれから決める)
    private(set) var latest: ChatTurn?
    private(set) var phase: Phase = .chatting
    /// 完成して中央に当てたアプリの id(完成の画面の「元に戻す」・閉じた後の要約のお知らせ・#968)
    private(set) var resultAppId: String?
    private(set) var sending = false
    /// 失敗の状態カード(#968・会話の下に出す)。分け方と文言は iOS と同じ `ErrorPresentation`
    private(set) var failure: ErrorPresentation?
    var draft: String
    /// 声で頼む(⇧⌘M・#966)で開いた。シートが出たらすぐ録音を始める(1 回だけ)
    var startsWithVoice = false
    /// 画面マップの「この画面について話す」(#970)で開いた時の画面。直す時は作る指示の先頭に「画面「B」(id: b)で: 」を付ける
    /// (iOS の `runFromChat` と同じ・会話の吹き出しには付けない)。会話のチップの × で外せる
    var screen: ScreenContext?
    /// 複数選択の question で選んだチップ・proposal で選んだ案の id
    var selectedChips: [String] = []
    var selectedOptions: [String] = []

    /// 設定の「対話」(#973)。開いた時に読む(iOS の `ClarifySettings` と同じ保存先)
    @ObservationIgnored private let clarifyEnabled: Bool
    @ObservationIgnored private let clarifyAutoBuild: Bool
    /// 最後に吹き出しを足した入力手段(自動で作るかの判定・#160)。声で頼む(#966)は `.voice` を渡す
    @ObservationIgnored private var lastInput: ClarifyFlow.InputSource?
    /// 要約カードを見せてから自動で作る待ち(#160)
    private(set) var autoBuildPending = false
    @ObservationIgnored private var autoBuildTask: Task<Void, Never>?

    /// 同意を待って止めた 1 件(iOS の `utteranceAwaitingConsent`)。同意したら送り直す・「あとで」なら捨てる
    enum Held: Equatable {
        case message(String)
        case build(String)
    }
    private(set) var heldForConsent: Held?
    /// 同意の説明を出す(ComposeSheet がシートで出す)
    var needsConsent = false

    @ObservationIgnored private let ai: any AIClient
    @ObservationIgnored private let appId: String
    @ObservationIgnored private let historyStore: ChatHistoryStore
    @ObservationIgnored private let reduceMotion: () -> Bool
    @ObservationIgnored private let isConsentGranted: @MainActor () -> Bool
    @ObservationIgnored private var task: Task<Void, Never>?
    /// 結果を中央に当てる(MacRootView がアプリの一覧を差し替える)
    /// 2 つ目は端末に残す形(封筒・#969)
    @ObservationIgnored var onResult: (AppDocument, Data?) -> Void = { _, _ in }
    /// 競合(409)の時にサーバーの最新を取り直して中央に当てる(#1279)。取り直した版を返す。nil ならサーバーに無いアプリ
    @ObservationIgnored var onConflict: (@MainActor () async -> Int?)?
    /// 直すアプリの今の版(競合で取り直したら進める)
    @ObservationIgnored private var editVersion: Int

    var mode: ChatMode {
        if case .edit = target { return .edit }
        return .create
    }

    /// 送っている・作っている間はシートを外側の操作で閉じない(Esc・閉じるボタンは使える)
    var isBusy: Bool {
        if sending { return true }
        switch phase {
        case .generating, .revealing: return true
        case .chatting, .finished: return false
        }
    }

    var offer: ClarifyFlow.Offer { latest.map(ClarifyFlow.offer(for:)) ?? .none }
    var shortcut: ClarifyFlow.Shortcut { bubbles.isEmpty ? .none : ClarifyFlow.shortcut(for: latest, mode: mode) }

    init(target: Target, draft: String = "", ai: any AIClient = MacComposeSession.gatedAI(),
         reduceMotion: @escaping () -> Bool = { NSWorkspace.shared.accessibilityDisplayShouldReduceMotion },
         isConsentGranted: @escaping @MainActor () -> Bool = { AIConsentStore.shared.isGranted },
         clarifyEnabled: Bool = ClarifySettings.enabled, clarifyAutoBuild: Bool = ClarifySettings.autoBuild,
         historyStore: ChatHistoryStore = .forRuntime()) {
        self.target = target
        self.draft = draft
        self.ai = ai
        self.reduceMotion = reduceMotion
        self.isConsentGranted = isConsentGranted
        self.clarifyEnabled = clarifyEnabled
        self.clarifyAutoBuild = clarifyAutoBuild
        self.historyStore = historyStore
        switch target {
        case .create:
            appId = "mac-\(UUID().uuidString.prefix(8).lowercased())"
            editVersion = 0
        case .edit(let id, _, let version):
            appId = id
            editVersion = version
        }
        history = historyStore.load(appId: appId)
    }

    /// Mac の AI。同意が無ければどの入口からも送らないよう `ConsentGatedAIClient` で包む(最後の砦・iOS の MetamonApp と同じ)
    nonisolated static func gatedAI(base: any AIClient = MacServices.shared.ai) -> any AIClient {
        ConsentGatedAIClient(base: base) { AIConsentStore.isGranted(in: .standard) }
    }

    // MARK: - 同意(#246)

    /// AI に送る直前に呼ぶ。未同意なら 1 件を止めて同意の説明を出し、false(送らない)
    private func requireConsent(_ held: Held) -> Bool {
        if isConsentGranted() { return true }
        heldForConsent = held
        needsConsent = true
        return false
    }

    /// 同意の説明が閉じた時。同意していれば止めた 1 件を送る。「あとで」なら送らず、書いた文は入力欄に戻す
    func resumeAfterConsent() {
        needsConsent = false
        guard let held = heldForConsent else { return }
        heldForConsent = nil
        guard isConsentGranted() else {
            // 書いた文は入力欄に戻す(相談 OFF で 1 発目をそのまま作ろうとした時も)
            switch held {
            case .message(let text) where draft.isEmpty: draft = text
            case .build(let text) where draft.isEmpty && bubbles.isEmpty: draft = text
            default: break
            }
            return
        }
        switch held {
        case .message(let text): send(text, source: lastInput ?? .text)
        case .build(let transcript): build(transcript)
        }
    }

    // MARK: - 会話

    /// 入力欄の文を送る(Return)。声で頼んだ文(#966)は `.voice`(自動で作るかの判定に使う)
    func sendDraft(source: ClarifyFlow.InputSource = .text) {
        let text = draft.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !text.isEmpty else { return }
        draft = ""
        send(text, source: source)
    }

    /// 1 発言を送る。1 発目は `ClarifyFlow.startConversation`(前の会話に混ざらない)、2 発目からは全履歴。
    /// 設定の「チャットで相談してから作る」が OFF なら、1 発目をそのまま作る・直す(iOS と同じ・一発で反映)。
    func send(_ text: String, source: ClarifyFlow.InputSource = .text) {
        guard !sending, phase == .chatting else { return }
        lastInput = source
        if !clarifyEnabled, bubbles.isEmpty { return build(text) }
        guard requireConsent(.message(text)) else { return }
        cancelAutoBuild()
        bubbles.append(Bubble(role: .user, text: text))
        persistHistory()
        selectedChips = []
        selectedOptions = []
        failure = nil
        sending = true
        let context = contextMessages
        let mode = mode
        task = Task { [ai, appId] in
            defer { sending = false }
            do {
                let turn = context.count == 1
                    ? try await ClarifyFlow.startConversation(ai: ai, appId: appId, first: context[0], locale: Self.locale, mode: mode).turn
                    : try await ai.chat(appId: appId, messages: context, locale: Self.locale, mode: mode)
                guard !Task.isCancelled else { return }
                latest = turn
                bubbles.append(Bubble(role: .assistant, text: turn.text, kind: turn.kind))
                persistHistory()
                MacVoiceOver.announce(turn.text)
                scheduleAutoBuildIfNeeded(turn)
            } catch {
                guard !Task.isCancelled else { return }
                // 届かなかった発言は履歴から外して入力欄に戻す(送り直しで同じ文が 2 回入らないように)
                if bubbles.last?.role == .user { bubbles.removeLast(); persistHistory() }
                // 送る途中で同意を取り消した時(包みが止めた): 同意の説明を出して、同意したら送り直す
                if error as? AIConsentError == .notGranted { _ = requireConsent(.message(text)); return }
                if draft.isEmpty { draft = text }
                show(Self.presentation(for: error, transcript: text, origin: .chat))
            }
        }
    }

    /// 声だけで進めていて ready が来たら、要約カードを少し見せてから作る(#160・create だけ・iOS と同じ判定)。
    /// その間に送る・ボタンを押す・閉じると取り消す。
    private func scheduleAutoBuildIfNeeded(_ turn: ChatTurn) {
        guard ClarifyFlow.shouldAutoBuild(turn: turn, lastInput: lastInput, enabled: clarifyAutoBuild, mode: mode) else { return }
        autoBuildPending = true
        autoBuildTask = Task { [weak self] in
            try? await Task.sleep(for: ClarifyFlow.autoBuildDelay)
            guard let self, !Task.isCancelled, self.autoBuildPending else { return }
            self.autoBuildPending = false
            self.autoBuildTask = nil
            self.confirm()
        }
    }

    private func cancelAutoBuild() {
        autoBuildTask?.cancel()
        autoBuildTask = nil
        autoBuildPending = false
    }

    /// チップを押す。複数選択の question は選ぶだけ(「決定」で送る)、それ以外はそのまま答えとして送る。
    func tapChip(_ chip: String) {
        guard latest?.isMultiSelect == true, latest?.kind == .question else { return send(chip, source: .chip) }
        if let i = selectedChips.firstIndex(of: chip) { selectedChips.remove(at: i) } else { selectedChips.append(chip) }
    }

    func submitChips() {
        guard MultiSelect.canSubmit(selectedCount: selectedChips.count) else { return }
        send(MultiSelect.joinedAnswer(selectedChips), source: .chip)
    }

    func toggleOption(_ option: ChatOption) {
        if let i = selectedOptions.firstIndex(of: option.id) { selectedOptions.remove(at: i) } else { selectedOptions.append(option.id) }
    }

    /// 案の「これでいく」。複数選択なら選んだ案(並びは案の順)を 1 つにまとめる。
    func choose(_ options: [ChatOption]) {
        guard let option = MultiSelect.combined(options) else { return }
        build(Self.instruction(requiresPro: option.requiresPro, full: option.instruction, free: option.freeInstruction))
    }

    /// 要約カードの「これでいく」
    func confirm() {
        guard case .confirm(let transcript) = offer, let turn = latest else { return }
        let full = transcript ?? bubbles.filter { $0.role == .user }.map(\.text).joined(separator: "。")
        build(Self.instruction(requiresPro: turn.requiresPro, full: full, free: turn.freeTranscript))
    }

    /// 「このまま作る」(create: go-ahead を送って ready を受ける)・「このまま反映」(edit: これまでの指示で直す)
    func runShortcut() {
        switch shortcut {
        case .none: return
        case .buildNow: send(L10n.tr("このまま作って"))
        case .applyNow: build(bubbles.filter { $0.role == .user }.map(\.text).joined(separator: "。"))
        }
    }

    /// 有料機能(#77)は Mac ではまだ買えない(課金は後の Issue)ので、無料版の指示があればそちらで作る。
    static func instruction(requiresPro: Bool?, full: String, free: String?) -> String {
        guard requiresPro == true, let free, !free.isEmpty else { return full }
        return free
    }

    /// 有料機能を外して作ることになるか(要約カード・案に一言出す)
    var dropsPaidFeatures: Bool { latest?.requiresPro == true && latest?.freeTranscript != nil }

    // MARK: - 生成

    func build(_ transcript: String) {
        guard phase == .chatting, !transcript.isEmpty, requireConsent(.build(transcript)) else { return }
        cancelAutoBuild()
        task?.cancel()
        sending = false
        failure = nil
        let job = Job(transcript: transcript, started: .now, script: .init(transcript: transcript))
        withAnimation { phase = .generating(job) }
        MacVoiceOver.announce(L10n.tr("作っています"))
        let version = editVersion
        let instruction = Self.instruction(transcript, about: mode == .edit ? screen : nil)
        task = Task { [ai, appId] in
            do {
                let result = try await ai.morph(transcript: instruction, appId: appId, currentVersion: version,
                                                locale: Self.locale) { progress in
                    Task { @MainActor [weak self] in self?.apply(progress) }
                }
                guard !Task.isCancelled else { return }
                await finish(result, job: job)
            } catch {
                guard !Task.isCancelled else { return }
                withAnimation { phase = .chatting }
                if error as? AIConsentError == .notGranted { _ = requireConsent(.build(transcript)); return }
                if FailureKind.classify(error) == .conflict, let onConflict, let latest = await onConflict() {
                    // 別の端末で直されていた: 最新に合わせ、カードの「もう一度」は取り直した版で送り直す(iOS のバナーと同じ文言)
                    editVersion = latest
                    show(ErrorPresentation(kind: .failed(message: ErrorCopy.conflictBanner), transcript: transcript, origin: .generate))
                    return
                }
                show(Self.presentation(for: error, transcript: transcript, origin: .generate))
            }
        }
    }

    /// 画面マップの文脈を付けた作る指示(#970)。文脈が無ければそのまま
    static func instruction(_ transcript: String, about screen: ScreenContext?) -> String {
        guard let screen else { return transcript }
        return ScreenMapCopy.contextPrefix(screen) + transcript
    }

    private func apply(_ progress: GenerateProgress) {
        guard case .generating(var job) = phase else { return }
        job.live.apply(progress)
        phase = .generating(job)
    }

    /// 結果が届いた。create は本物の部品を上から置いてから(#626)、edit はすぐに中央を差し替える
    private func finish(_ result: MorphResult, job: Job) async {
        var document = result.document
        document.appId = appId
        let summary = result.summary ?? L10n.tr("できあがりました")
        let done: () -> Void = { [weak self] in
            self?.complete(document, envelope: result.envelope, summary: summary, unchanged: result.unchanged)
        }
        guard mode == .create, !result.unchanged else { done(); return }
        let plan = BuildChoreography.AssemblyPlan.make(for: document.dsl, reduceMotion: reduceMotion(), shown: job.live.parts)
        phase = .revealing(job, plan)
        pendingReveal = done
        try? await Task.sleep(for: .seconds(plan.total))
        guard !Task.isCancelled else { return }
        skipReveal()
    }

    /// 組み上げを終える(時間切れ・クリックで飛ばす)
    func skipReveal() {
        guard case .revealing = phase, let done = pendingReveal else { return }
        pendingReveal = nil
        task?.cancel()
        done()
    }

    private func complete(_ document: AppDocument, envelope: Data?, summary: String, unchanged: Bool) {
        noteApplied(version: document.version, summary: summary, unchanged: unchanged)
        if !unchanged {
            onResult(document, envelope)
            resultAppId = document.appId
        }
        withAnimation { phase = .finished(summary: summary, unchanged: unchanged) }
        MacVoiceOver.announce(summary)
    }

    /// 生成中の「やめる」。会話に戻る(中央は変えない)
    func cancelBuild() {
        task?.cancel()
        withAnimation { phase = .chatting }
    }

    // MARK: - 会話の履歴(#1211・iOS の `AppRuntime+ChatHistory`)

    /// 閉じた会話があるか
    var hasHistory: Bool { !history.isEmpty }

    /// AI に送る messages(過去の変更の要約 + 最後の反映以降 + 今の会話・iOS と同じ `ChatContext`)
    var contextMessages: [ChatMessage] {
        ChatContext.messages(history: history, live: bubbles.map { ChatMessage(role: $0.role, text: $0.text) })
    }

    /// 今の質問が何問目か(「質問 n / 3」)。考えている間は出さない
    var questionNumber: Int? {
        sending ? nil : QuestionProgress.number(bubbles: bubbles, mode: mode, latest: latest?.kind)
    }

    /// 反映した所で今の会話を閉じ、区切りを入れる。対話を使っていない(履歴が空)なら入れない。変えられなかった時は会話だけ閉じる
    private func noteApplied(version: Int, summary: String, unchanged: Bool) {
        history = ChatHistoryStore.trimmed(history + liveEntries)
        bubbles = []
        latest = nil
        if !unchanged, hasHistory {
            history = ChatHistoryStore.trimmed(history + [ChatHistoryEntry(kind: .applied, text: summary, version: version)])
        }
        persistHistory()
    }

    /// 「最初から」。これまでの会話があれば確認を出す(iOS の M9)。無ければ今の会話だけ消す
    func requestRestart() {
        guard phase == .chatting, !sending else { return }
        if hasHistory { showsRestartConfirm = true } else { restartLive() }
    }

    /// 今の会話だけ消す(これまでの会話と区切りは残す)
    func restartLive() {
        showsRestartConfirm = false
        resetLive()
        persistHistory()
    }

    /// これまでの会話も消す(戻せない)
    func restartAll() {
        showsRestartConfirm = false
        resetLive()
        history = []
        historyStore.remove(appId: appId)
    }

    private func resetLive() {
        cancelAutoBuild()
        task?.cancel()
        sending = false
        bubbles = []
        latest = nil
        selectedChips = []
        selectedOptions = []
        failure = nil
        // サーバーの会話は消しに行かない: 次の 1 発目は `ClarifyFlow.startConversation` が古い会話を検出してやり直す(iOS と同じ)
    }

    private func persistHistory() {
        historyStore.save(history + liveEntries, appId: appId)
    }

    private var liveEntries: [ChatHistoryEntry] {
        bubbles.map { ChatHistoryEntry(kind: $0.role == .user ? .user : .assistant, text: $0.text) }
    }

    // MARK: - 失敗(#968)

    /// 失敗 → 状態カード。iOS の `ErrorPresentation.from` と同じ分け方。キャンセルは出さない。
    /// 競合(409)は、サーバーにあるアプリなら `onConflict` で最新を取り直してから「もう一度」のカード(#1279)。
    /// サーバーに無いアプリ(端末だけ)は今まで通り「もう一度」のカード。
    static func presentation(for error: Error, transcript: String, origin: ErrorPresentation.Origin) -> ErrorPresentation? {
        let kind = FailureKind.classify(error)
        if kind == .cancelled { return nil }
        if (error as? APIError)?.isTextNotAllowed == true {
            ErrorLog.record(error, context: origin == .chat ? "mac chat" : "mac morph")
            return ErrorPresentation(kind: .failed(message: ErrorCopy.textNotAllowed), transcript: transcript, origin: origin)
        }
        return ErrorPresentation.from(kind, transcript: transcript, origin: origin)
            ?? ErrorPresentation(kind: .failed(message: ErrorCopy.otherBody), transcript: transcript, origin: origin)
    }

    private func show(_ presentation: ErrorPresentation?) {
        guard let presentation else { return }
        withAnimation { failure = presentation }
    }

    /// カードの「もう一度」。会話の失敗は入力欄に戻した文(直していればその文)を送り直し、生成の失敗は同じ指示で作り直す
    func retryFailure() {
        guard let failure, phase == .chatting, !sending else { return }
        switch failure.origin {
        case .chat:
            if draft.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty { send(failure.transcript) } else { sendDraft() }
        case .generate:
            build(failure.transcript)
        }
    }

    /// カードの「閉じる」
    func dismissFailure() { withAnimation { failure = nil } }

    /// シートを閉じる時。走っている呼び出しを止める
    func close() {
        cancelAutoBuild()
        task?.cancel()
    }

    @ObservationIgnored private var pendingReveal: (() -> Void)?

    /// 生成アプリの文言の言語。iOS の `AppRuntime.generationLocale` と同じ(システムに任せる時は nil = 発話から判定)
    private static var locale: String? {
        let pref = LanguagePreference.shared
        return pref.setting == .system ? nil : pref.code
    }
}
