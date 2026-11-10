import SwiftUI
import AppKit
import Observation
import MetamonRenderer
import MetamonDSL
import MetamonServices

/// アプリを作る・直す会話(#967)の状態。iOS の `AppRuntime+Clarify` / `AppRuntime+BuildStage` の Mac 版。
/// 判断は iOS と同じ `ClarifyFlow`(何を出すか)・`MultiSelect`(複数選択)・`BuildChoreography`(組み上げ)を使う。
/// AI は Mac に Firebase が無い間(#930)は `MockAIClient`(iOS の METAMON_MOCK と同じ・同梱の DSL 例を返す)。
/// 流れ: 頼む(文字)→ 聞き返し・案(chat)→「これでいく」→ 生成中(組み立て台・経過秒)→ 組み上げ → 完成の要約。
@MainActor
@Observable
final class MacComposeSession: Identifiable {
    /// 何を相手に話しているか。edit は今中央で開いているアプリを直す
    enum Target: Equatable {
        case create
        case edit(appId: String, name: String, version: Int)
    }

    struct Bubble: Identifiable, Equatable {
        let id = UUID()
        var role: ChatMessage.Role
        var text: String
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
    /// 最後の応答(チップ・案・要約カードはこれから決める)
    private(set) var latest: ChatTurn?
    private(set) var phase: Phase = .chatting
    private(set) var sending = false
    /// 失敗した時の一言(会話の下に出す)
    private(set) var error: String?
    var draft: String
    /// 複数選択の question で選んだチップ・proposal で選んだ案の id
    var selectedChips: [String] = []
    var selectedOptions: [String] = []

    @ObservationIgnored private let ai: any AIClient
    @ObservationIgnored private let appId: String
    @ObservationIgnored private let reduceMotion: () -> Bool
    @ObservationIgnored private var task: Task<Void, Never>?
    /// 結果を中央に当てる(MacRootView がアプリの一覧を差し替える)
    @ObservationIgnored var onResult: (AppDocument) -> Void = { _ in }

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

    init(target: Target, draft: String = "", ai: any AIClient = MockAIClient(),
         reduceMotion: @escaping () -> Bool = { NSWorkspace.shared.accessibilityDisplayShouldReduceMotion }) {
        self.target = target
        self.draft = draft
        self.ai = ai
        self.reduceMotion = reduceMotion
        switch target {
        case .create: appId = "mac-\(UUID().uuidString.prefix(8).lowercased())"
        case .edit(let id, _, _): appId = id
        }
    }

    // MARK: - 会話

    /// 入力欄の文を送る(Return)
    func sendDraft() {
        let text = draft.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !text.isEmpty else { return }
        draft = ""
        send(text)
    }

    /// 1 発言を送る。1 発目は `ClarifyFlow.startConversation`(前の会話に混ざらない)、2 発目からは全履歴。
    func send(_ text: String) {
        guard !sending, phase == .chatting else { return }
        let first = bubbles.isEmpty
        bubbles.append(Bubble(role: .user, text: text))
        selectedChips = []
        selectedOptions = []
        error = nil
        sending = true
        let history = bubbles.map { ChatMessage(role: $0.role, text: $0.text) }
        let mode = mode
        task = Task { [ai, appId] in
            defer { sending = false }
            do {
                let turn = first
                    ? try await ClarifyFlow.startConversation(ai: ai, appId: appId, first: history[0], locale: Self.locale, mode: mode).turn
                    : try await ai.chat(appId: appId, messages: history, locale: Self.locale, mode: mode)
                guard !Task.isCancelled else { return }
                latest = turn
                bubbles.append(Bubble(role: .assistant, text: turn.text))
                MacVoiceOver.announce(turn.text)
            } catch {
                guard !Task.isCancelled else { return }
                // 届かなかった発言は履歴から外して入力欄に戻す(送り直しで同じ文が 2 回入らないように)
                if bubbles.last?.role == .user { bubbles.removeLast() }
                if draft.isEmpty { draft = text }
                self.error = L10n.tr("うまく届きませんでした。もう一度送ってください")
            }
        }
    }

    /// チップを押す。複数選択の question は選ぶだけ(「決定」で送る)、それ以外はそのまま答えとして送る。
    func tapChip(_ chip: String) {
        guard latest?.isMultiSelect == true, latest?.kind == .question else { return send(chip) }
        if let i = selectedChips.firstIndex(of: chip) { selectedChips.remove(at: i) } else { selectedChips.append(chip) }
    }

    func submitChips() {
        guard MultiSelect.canSubmit(selectedCount: selectedChips.count) else { return }
        send(MultiSelect.joinedAnswer(selectedChips))
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
        guard phase == .chatting, !transcript.isEmpty else { return }
        task?.cancel()
        sending = false
        error = nil
        let job = Job(transcript: transcript, started: .now, script: .init(transcript: transcript))
        withAnimation { phase = .generating(job) }
        MacVoiceOver.announce(L10n.tr("作っています"))
        let version: Int = if case .edit(_, _, let v) = target { v } else { 0 }
        task = Task { [ai, appId] in
            do {
                let result = try await ai.morph(transcript: transcript, appId: appId, currentVersion: version,
                                                locale: Self.locale) { progress in
                    Task { @MainActor [weak self] in self?.apply(progress) }
                }
                guard !Task.isCancelled else { return }
                await finish(result, job: job)
            } catch {
                guard !Task.isCancelled else { return }
                withAnimation { phase = .chatting }
                self.error = L10n.tr("作れませんでした。言い方を変えて、もう一度頼んでください")
            }
        }
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
        let done: () -> Void = { [weak self] in self?.complete(document, summary: summary, unchanged: result.unchanged) }
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

    private func complete(_ document: AppDocument, summary: String, unchanged: Bool) {
        if !unchanged { onResult(document) }
        withAnimation { phase = .finished(summary: summary, unchanged: unchanged) }
        MacVoiceOver.announce(summary)
    }

    /// 生成中の「やめる」。会話に戻る(中央は変えない)
    func cancelBuild() {
        task?.cancel()
        withAnimation { phase = .chatting }
    }

    /// シートを閉じる時。走っている呼び出しを止める
    func close() { task?.cancel() }

    @ObservationIgnored private var pendingReveal: (() -> Void)?

    /// 生成アプリの文言の言語。iOS の `AppRuntime.generationLocale` と同じ(システムに任せる時は nil = 発話から判定)
    private static var locale: String? {
        let pref = LanguagePreference.shared
        return pref.setting == .system ? nil : pref.code
    }
}
