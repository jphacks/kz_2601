// 審査用ビルド(APP_STORE_REVIEW)ではバイナリから外す(オンライン AI = POST /v1/text を持たない・#452 G4)。
#if !APP_STORE_REVIEW
import SwiftUI
import MetamonDSL

/// `aiChat`: クラウドの AI と何往復も話す。吹き出しは古い順に上から下へ(自分は右・AI は左)、下に固定の入力欄。
/// 送る = ホストの `generateAIText`(task chat・同意 → Pro の確認もホスト)。発言と返事を `entity` に 1 件ずつ保存する。
/// 自分の発言を先に保存する(返事に失敗しても残る)。roleField の無い古い DSL(stg)は役を交互の並びで読むので、
/// 返事が来てから 2 件を対で保存する(片方が保存できない・断られたらもう片方も消す = `AIChatPairSaving`)。
/// 読み上げ: AI の吹き出しごとに小さなスピーカー(`speakButton` と同じ `LeafSpeech`・端末の声)。`speak: true` なら届いた返事を自動で読む。画面を離れたら止める。
/// 話して入力: 入力欄の横のマイク(`transcribeField` と同じ `DictationHost`・端末内)。押して始め、押して止めると入力欄に文字が入る。送るのはユーザー。
/// 録音を始めると読み上げを止め、録音の間は返事を自動で読まない。画面を離れた・マイク FAB の音声入力が始まったら録音を捨てる。
struct AiChatLeaf: View {
    @Environment(\.dslRuntime) private var rt
    @Environment(\.fabFrame) private var fabFrame
    let c: Component
    let ctx: RenderContext

    @State private var draft = ""
    @State private var sending = false
    /// 返事を待っている自分の発言(roleField が無く、まだ保存していない時だけ)
    @State private var pending: String?
    @State private var error: String?
    @State private var echo: SentTextEcho?
    @State private var barFrame: CGRect = .zero
    @State private var scrollFrame: CGRect = .zero
    @FocusState private var focused: Bool
    /// 読み上げの持ち主の頭(吹き出しは `owner/turnId`)。
    @State private var owner = UUID().uuidString
    /// 画面に出ているか。送信の Task は画面を離れても続くので、離れた後に届いた返事は保存だけして読まない
    @State private var visible = false
    @State private var micPhase: DictationPhase = .idle
    /// saveTo の記録の案(最新の返事の分だけ・会話の記録には残さない・AiChatSuggestCard.swift)
    @State private var suggestion: AiChatSuggestion?
    private var speech: LeafSpeech { .shared }

    private var online: Bool { rt.aiText?.supportsOnlineAIText ?? false }

    var body: some View {
        if let entityId = c.entity, let textField = c.textField, let entity = rt.dsl.entity(entityId) {
            VStack(spacing: RenderTheme.Spacing.sm) {
                messages(entity: entity, textField: textField)
                footer
                inputBar(entity: entity, textField: textField)
            }
            .onAppear { visible = true }
            .onDisappear {
                visible = false
                stopSpeech()
                cancelDictation()
            }
            .onChange(of: rt.dictation?.isVoiceInputActive ?? false) { _, active in
                if active { cancelDictation() }
            }
        } else {
            UnsupportedComponent(label: "aiChat")
        }
    }

    private func speechOwner(_ turnId: String) -> String { "\(owner)/\(turnId)" }

    private func toggleSpeech(_ t: AIChat.Turn) {
        let id = speechOwner(t.id)
        if speech.speakingOwner == id { speech.stop(owner: id) } else { speech.speak(t.text, owner: id) }
    }

    /// この部品の吹き出しが読んでいれば止める(別の部品の読み上げには触らない)。
    private func stopSpeech() {
        if let s = speech.speakingOwner, s.hasPrefix(owner + "/") { speech.stop(owner: s) }
    }

    // MARK: - 吹き出し

    @ViewBuilder
    private func messages(entity: Entity, textField: String) -> some View {
        let turns = AIChat.turns(ownRecords(entity), textField: textField, roleField: c.roleField)
        if turns.isEmpty, pending == nil, !sending {
            EmptyPlaceholder(icon: "sparkles", emptyText: c.emptyText ?? L("話しかけてみましょう"),
                             state: rt.emptyState(for: entity.id)) {
                Task { await rt.load(entities: [entity.id], force: true) }
            }
            .frame(maxWidth: .infinity, minHeight: 200, maxHeight: .infinity)
        } else {
            ScrollViewReader { proxy in
                ScrollView {
                    LazyVStack(spacing: RenderTheme.Spacing.xs) {
                        ForEach(turns, id: \.id) { t in
                            if t.role == .user {
                                AiChatBubble(text: t.text, mine: true).id(t.id)
                            } else {
                                AiChatBubble(text: t.text, mine: false, speaking: speech.speakingOwner == speechOwner(t.id),
                                             canSpeak: AiChatMic.canSpeak(phase: micPhase)) {
                                    toggleSpeech(t)
                                }
                                .id(t.id)
                                if let s = suggestion, s.turnId == t.id {
                                    AiChatSuggestCard(suggestion: s, onAdd: addSuggestion, onFix: fixSuggestion)
                                        .id("suggest")
                                }
                            }
                        }
                        if let pending { AiChatBubble(text: pending, mine: true).id("pending") }
                        if sending { AiChatThinking().id("thinking") }
                    }
                    .padding(.vertical, RenderTheme.Spacing.xs)
                    .padding(.bottom, FabClearance.contentMargin(viewport: scrollFrame, fab: fabFrame))
                }
                .frame(minHeight: 200)
                .onGeometryChange(for: CGRect.self) { $0.frame(in: .global) } action: { scrollFrame = $0 }
                .defaultScrollAnchor(.bottom)
                .scrollDismissesKeyboard(.interactively)
                .onChange(of: turns.last?.id) { _, _ in scrollToEnd(proxy, turns) }
                .onChange(of: sending) { _, _ in scrollToEnd(proxy, turns) }
                .onChange(of: suggestion?.turnId) { _, _ in scrollToEnd(proxy, turns) }
                .onChange(of: focused) { _, isFocused in if isFocused { scrollToEnd(proxy, turns) } }
            }
            .accessibilityIdentifier("aiChat.messages")
        }
    }

    /// 表示と送る履歴に使う記録(自分の分だけ・`AIChat.own`)。
    private func ownRecords(_ entity: Entity) -> [Record] {
        AIChat.own(rt.viewRecords(for: entity.id), uid: rt.currentUserId)
    }

    private func scrollToEnd(_ proxy: ScrollViewProxy, _ turns: [AIChat.Turn]) {
        let tail = suggestion != nil && suggestion?.turnId == turns.last?.id ? "suggest" : turns.last?.id
        let last: String? = sending ? "thinking" : (pending != nil ? "pending" : tail)
        guard let last else { return }
        withAnimation(RenderTheme.Motion.appear) { proxy.scrollTo(last, anchor: .bottom) }
    }

    @ViewBuilder private var footer: some View {
        if let error {
            Text(error).font(.caption).foregroundStyle(.red).frame(maxWidth: .infinity, alignment: .leading)
                .accessibilityIdentifier("aiChat.error")
        } else if !online {
            Text(L("この版では AI と話せません")).font(.caption).foregroundStyle(.secondary).frame(maxWidth: .infinity, alignment: .leading)
        }
    }

    // MARK: - 入力欄(画面の下に固定)

    private func inputBar(entity: Entity, textField: String) -> some View {
        HStack(alignment: .bottom, spacing: RenderTheme.Spacing.sm) {
            TextField(c.placeholder ?? L("メッセージ"), text: $draft, axis: .vertical)
                .lineLimit(1...4)
                .textFieldStyle(.plain)
                .font(RenderTheme.Fonts.inputValue)
                .focused($focused)
                .keyboardAnchor(c, $focused)
                .accessibilityIdentifier("aiChat.input")
                .onChange(of: draft) { _, new in dropEcho(new) }
                .renderInputChrome(focused: focused)
            micButton
            Button { send(entity: entity, textField: textField) } label: {
                Image(systemName: "arrow.up.circle.fill").font(.system(size: 34)).frame(height: 44)
            }
            .buttonStyle(.plain)
            .foregroundStyle(canSend ? AnyShapeStyle(RenderTheme.brandFill) : AnyShapeStyle(.tertiary))
            .disabled(!canSend)
            .accessibilityLabel(L("送信"))
            .accessibilityIdentifier("aiChat.send")
        }
        .padding(.leading, ChatInputInset.leading(bar: barFrame, fab: fabFrame))
        .onGeometryChange(for: CGRect.self) { $0.frame(in: .global) } action: { barFrame = $0 }
        .padding(.bottom, focused ? KeyboardState.toolbarClearance : 0)
    }

    private var canSend: Bool {
        AiChatMic.canSend(online: online, sending: sending, draft: draft, phase: micPhase)
    }

    // MARK: - 話して入力

    private var mic: AiChatMic {
        let host = rt.dictation
        return AiChatMic.state(phase: micPhase, readiness: host?.dictationReadiness,
                               voiceInputActive: host?.isVoiceInputActive ?? false, online: online)
    }

    @ViewBuilder private var micButton: some View {
        let mic = mic
        if mic != .hidden {
            Button { micTapped(mic) } label: {
                Group {
                    switch mic {
                    case .transcribing: ProgressView()
                    case .stop: Image(systemName: "stop.circle.fill").symbolEffect(.pulse)
                    case .prepare: Image(systemName: "arrow.down.circle")
                    default: Image(systemName: "mic.circle")
                    }
                }
                .font(.system(size: 30))
                .frame(minWidth: 44, minHeight: 44)
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .foregroundStyle(mic == .stop ? AnyShapeStyle(.red) : (mic.enabled ? AnyShapeStyle(RenderTheme.brandFill) : AnyShapeStyle(.tertiary)))
            .disabled(!mic.enabled)
            .accessibilityLabel(Self.micLabel(mic))
            .accessibilityHint(mic == .start ? L("話した言葉が入力欄に入ります。確かめてから送れます") : "")
            .accessibilityIdentifier("aiChat.mic")
        }
    }

    static func micLabel(_ mic: AiChatMic) -> String {
        switch mic {
        case .hidden, .start: L("話して入力")
        case .stop: L("聞いています… タップで終わる")
        case .transcribing: L("文字にしています…")
        case .prepare: L("音声モデルを用意する")
        case .busy: L("今は話して入力を使えません")
        }
    }

    private func micTapped(_ mic: AiChatMic) {
        switch mic {
        case .start: Task { await startDictation() }
        case .stop: Task { await finishDictation() }
        case .prepare: rt.dictation?.prepareDictation()
        case .hidden, .transcribing, .busy: break
        }
    }

    private func startDictation() async {
        guard let host = rt.dictation, micPhase == .idle else { return }
        await KeyboardState.finishEditing() // 変換中の文字を確定させてから(入力欄に足すので)
        focused = false
        stopSpeech()
        error = nil
        do {
            try await host.startDictation() // 読み上げ(アプリ全体)を止め、録音の間は読まない
            guard visible, !host.isVoiceInputActive else { return host.cancelDictation() } // 始める間に画面を離れた
            micPhase = .listening
        } catch {
            self.error = error.localizedDescription
        }
    }

    private func finishDictation() async {
        guard let host = rt.dictation, micPhase == .listening else { return }
        micPhase = .transcribing
        do {
            let text = try await host.finishDictation()
            echo = nil
            draft = DictationText.appending(text, to: draft)
        } catch {
            self.error = error.localizedDescription
        }
        micPhase = .idle
    }

    private func cancelDictation() {
        guard micPhase == .listening else { return }
        rt.dictation?.cancelDictation()
        micPhase = .idle
    }

    private func send(entity: Entity, textField: String) {
        let text = ((focused ? KeyboardState.takeFocusedText() : nil) ?? draft).trimmingCharacters(in: .whitespacesAndNewlines)
        guard !text.isEmpty, let host = rt.aiText, !sending else { return }
        let roleField = c.roleField
        let base = ChatThreadLeaf.defaultData(entity, skipping: [textField, roleField].compactMap { $0 })
        let history = AIChat.turns(ownRecords(entity), textField: textField, roleField: roleField)
        let saveTo = c.chatSaveTo
        let dsl = self.rt.dsl
        let saveEntity = saveTo.flatMap { dsl.entity($0.entity) }
        let request = AIChat.request(persona: c.persona ?? "", history: history, next: text, long: c.length == "long",
                                     locale: LanguagePreference.shared.code,
                                     saveTo: saveEntity.flatMap { saveTo?.request(in: $0, today: DSLClock.now()) })
        sending = true
        error = nil
        suggestion = nil // 前の案は閉じる(言い直した内容で新しい案が来る)
        if focused { KeyboardState.clearFocusedText() }
        draft = ""
        echo = TextSubmission.value(text).map { SentTextEcho(sent: $0) }
        let rt = rt
        Task {
            defer { sending = false }
            if roleField != nil {
                // 自分の発言を先に残す(返事に失敗しても消えない・次の送信で続けて読まれる)
                guard let mine = AIChat.record(text: text, role: .user, textField: textField, roleField: roleField, base: base),
                      await rt.createRecord(entity: entity.id, data: mine) else {
                    restore(text)
                    self.error = L("会話を保存できませんでした")
                    return
                }
            } else {
                pending = text
            }
            do {
                let reply = try await host.generateAIText(request)
                pending = nil
                let saved = await Self.saveReply(reply.text, after: text, rt: rt, entity: entity.id, textField: textField,
                                                 roleField: roleField, base: base)
                if !saved {
                    if roleField == nil { restore(text) } // 対で残さなかった = 自分の発言も消えている
                    self.error = roleField == nil ? L("会話を保存できませんでした") : L("AI の返事を保存できませんでした")
                } else if let spoken = AIChat.spokenReply(AIChat.turns(ownRecords(entity), textField: textField, roleField: roleField),
                                                          reply: reply.text) {
                    // saveTo: 返事の下に記録の案(押すまで保存しない)
                    suggestion = AiChatSuggestion.make(turnId: spoken.id, suggest: reply.suggest, saveTo: saveTo,
                                                       entity: saveEntity, display: { displayText($0, field: $1) })
                    autoRead(spoken) // speak: true(保存できた返事だけ・録音中は読まない)
                }
            } catch {
                if roleField == nil { pending = nil; restore(text) }
                self.error = Self.message(error)
            }
        }
    }

    /// 届いた返事の自動の読み上げ(`speak: true`)。
    enum AutoRead: Equatable { case none, speak, announce }

    /// 画面を離れた後は読まない。VoiceOver が動いている時は端末の声と重ねず、VoiceOver に読ませる(スピーカーのボタンはそのまま)。
    /// 話して入力の録音・文字起こしの間も読まない(声をマイクが拾う・VoiceOver の読み上げも同じ)。
    static func autoReadMode(speak: Bool?, visible: Bool, voiceOver: Bool, recording: Bool = false) -> AutoRead {
        guard speak == true, visible, !recording else { return .none }
        return voiceOver ? .announce : .speak
    }

    private func autoRead(_ turn: AIChat.Turn) {
        switch Self.autoReadMode(speak: c.speak, visible: visible, voiceOver: UIAccessibility.isVoiceOverRunning,
                                 recording: micPhase != .idle) {
        case .none: break
        case .speak: speech.speak(turn.text, owner: speechOwner(turn.id))
        case .announce: UIAccessibility.post(notification: .announcement, argument: turn.text)
        }
    }

    /// 返事が届いた後の保存。保存できたら true。
    /// roleField がある: 自分の発言は保存済みなので返事だけ。無い: 自分 → AI を対で保存し、片方だけは残さない(役は並び順で読むため)。
    static func saveReply(_ reply: String, after text: String, rt: any DSLRuntime, entity: String, textField: String,
                          roleField: String?, base: [String: JSONValue]) async -> Bool {
        guard let ai = AIChat.record(text: reply, role: .assistant, textField: textField, roleField: roleField, base: base) else {
            return false // 空の返事
        }
        guard roleField == nil else { return await rt.createRecord(entity: entity, data: ai) }
        guard let mine = AIChat.record(text: text, role: .user, textField: textField, roleField: nil, base: base) else { return false }
        return await rt.saveTurnPair(entity: entity, user: mine, assistant: ai)
    }

    /// 記録の案の「追加」: いつもの保存(`createRecord`)で 1 件足す。entity の上限に達していれば足さない。
    private func addSuggestion() {
        guard var s = suggestion, s.state != .saving, s.state != .saved else { return }
        s.state = .saving
        suggestion = s
        let rt = rt
        Task {
            await rt.load(entities: [s.entity])
            let message: String?
            if AISaveTo.room(existing: rt.records(for: s.entity).count, wanted: 1) == 0 {
                message = L("これ以上記録を足せません")
            } else {
                message = await rt.createRecord(entity: s.entity, data: s.data) ? nil : L("記録できませんでした")
            }
            guard suggestion?.turnId == s.turnId else { return } // その間に次を送った
            suggestion?.state = message.map { .failed($0) } ?? .saved
            AccessibilityNotification.Announcement(message ?? L("記録しました")).post()
        }
    }

    /// 記録の案の「直す」: カードを閉じて入力欄へ(会話で言い直すと AI が新しい案を出す)。
    private func fixSuggestion() {
        suggestion = nil
        focused = true
    }

    /// 送れなかった文を入力欄に戻す(打ち直さなくてよい)。
    private func restore(_ text: String) {
        echo = nil
        if draft.isEmpty { draft = text }
    }

    /// 画面に出す文。Pro が要る時は aiChat の言い方にする(課金画面はホストが出す)。参加者には課金の案内を出さない(#704)。
    static func message(_ error: Error) -> String {
        switch error as? AITextError {
        case .needsPro?: L("AI と話すには Tailor It Pro が必要です")
        case .creatorNeedsPro?: L("今は AI と話せません。アプリを作った人に知らせてください")
        default: error.localizedDescription
        }
    }

    /// 送った直後にキーボードから書き戻された文字を消す(#627)。
    private func dropEcho(_ new: String) {
        guard let echo else { return }
        if echo.shouldClear(new) {
            KeyboardState.clearFocusedText()
            draft = ""
        } else if !new.isEmpty {
            self.echo = nil
        }
    }
}

/// 1 発言の吹き出し。自分は右にブランド色・AI は左に面の色(上に「AI」・下に読み上げのスピーカー)。
private struct AiChatBubble: View {
    let text: String
    let mine: Bool
    var speaking = false
    /// 話して入力の録音中は false(スピーカーを押せない)
    var canSpeak = true
    /// AI の吹き出しだけ: 読み上げ / 止める
    var onSpeak: (() -> Void)?

    var body: some View {
        HStack(alignment: .bottom, spacing: 0) {
            if mine { Spacer(minLength: 56) }
            VStack(alignment: mine ? .trailing : .leading, spacing: 3) {
                if !mine {
                    Label(L("AI"), systemImage: "sparkles").font(.caption.weight(.semibold)).foregroundStyle(.secondary)
                        .padding(.horizontal, 6).padding(.top, RenderTheme.Spacing.xs)
                        .accessibilityHidden(true) // 吹き出しのラベルが「AI, …」と読む
                }
                let shape = RoundedRectangle(cornerRadius: 18, style: .continuous)
                Text(text).font(.body).foregroundStyle(mine ? AnyShapeStyle(.white) : AnyShapeStyle(.primary))
                    .fixedSize(horizontal: false, vertical: true)
                    .textSelection(.enabled)
                    .padding(.horizontal, 14)
                    .padding(.vertical, 9)
                    .background(mine ? AnyShapeStyle(RenderTheme.brandFill) : AnyShapeStyle(RenderTheme.Colors.surfaceElevated), in: shape)
                    .overlay { if !mine { shape.strokeBorder(RenderTheme.Colors.hairline) } }
                    .accessibilityElement(children: .ignore)
                    .accessibilityLabel([mine ? L("自分") : L("AI"), text].joined(separator: ", "))
                    .accessibilityIdentifier(mine ? "aiChat.message.mine" : "aiChat.message.ai")
                if let onSpeak {
                    Button(action: onSpeak) {
                        Image(systemName: speaking ? "stop.fill" : "speaker.wave.2.fill")
                            .font(.footnote)
                            .frame(minWidth: 44, minHeight: 32, alignment: .leading)
                            .padding(.leading, 6)
                            .contentShape(Rectangle())
                    }
                    .buttonStyle(.plain)
                    .foregroundStyle(.secondary)
                    .disabled(!canSpeak && !speaking)
                    .accessibilityLabel(speaking ? L("止める") : L("読み上げる"))
                    .accessibilityIdentifier("aiChat.speak")
                }
            }
            if !mine { Spacer(minLength: 56) }
        }
    }
}

/// 返事を待っている間の左の吹き出し。
private struct AiChatThinking: View {
    var body: some View {
        HStack {
            HStack(spacing: RenderTheme.Spacing.sm) {
                ProgressView().controlSize(.small)
                Text(L("AI が考えています…")).font(.callout).foregroundStyle(.secondary)
            }
            .padding(.horizontal, 14)
            .padding(.vertical, 9)
            .background(RenderTheme.Colors.surfaceElevated, in: RoundedRectangle(cornerRadius: 18, style: .continuous))
            Spacer(minLength: 56)
        }
        .accessibilityIdentifier("aiChat.thinking")
    }
}
#endif
