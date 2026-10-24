import SwiftUI
import MetamonServices
import MetamonRenderer

/// 新規作成の前に質問を挟むかの設定(#75)。既定 ON。`METAMON_CLARIFY=0/1` があれば UserDefaults より優先(E2E・デモ端末用)。
enum ClarifySettings {
    private static let key = "metamon.clarify.enabled"

    static var enabled: Bool {
        get {
            switch ProcessInfo.processInfo.environment["METAMON_CLARIFY"] {
            case "0": return false
            case "1": return true
            default: return UserDefaults.standard.object(forKey: key) as? Bool ?? true
            }
        }
        set { UserDefaults.standard.set(newValue, forKey: key) }
    }

    private static let autoBuildKey = "metamon.clarify.autoBuild"

    /// 音声だけで進めている時、ready で自動生成するか(#160)。既定 ON(デモ本線は声だけで最後まで進むのが主役)。
    /// `METAMON_CLARIFY_AUTOBUILD=0/1` があれば優先。
    static var autoBuild: Bool {
        get {
            switch ProcessInfo.processInfo.environment["METAMON_CLARIFY_AUTOBUILD"] {
            case "0": return false
            case "1": return true
            default: return UserDefaults.standard.object(forKey: autoBuildKey) as? Bool ?? true
            }
        }
        set { UserDefaults.standard.set(newValue, forKey: autoBuildKey) }
    }
}

/// 生成前の対話(#75 / backend #74 `POST /v1/chat`)。
/// 初期(音声)画面で、最初の発話を即生成せず ai に聞き返させ、1〜3 ラリーで要件を固めてから `/v1/generate` に渡す。
/// - 会話状態は ai が appId ごとに持つ。iOS は手元の全履歴を毎回送る(2 件以上はサーバー側を置き換える契約)
/// - ready の `transcript` を `generate(_:)`(意図判定なし)に渡す。generate の契約は無変更
/// - 「このまま作る」は go-ahead 語を送って backend の shortcut ready に任せる(失敗してもこれまでの発話で生成する)
/// - edit(生成後の改変)は対話を挟まない
extension AppRuntime {
    /// generate / chat に送る `locale`(#45)。設定で言語を明示したらそれ(UI と生成アプリの言語を揃える)、
    /// 「自動」は nil = サーバーが発話の文字種から判定(日本語 UI でも英語で話せば英語のアプリになる)。
    static var generationLocale: String? {
        let pref = LanguagePreference.shared
        return pref.setting == .system ? nil : pref.code
    }

    /// 対話中か(吹き出しが 1 つ以上)。
    var isChatting: Bool { !chatBubbles.isEmpty }

    /// 音声画面からの発話の入口。初期画面 + 対話 ON + 通常の依頼(「元に戻して」等でない)なら対話へ、それ以外は従来どおり即生成。
    /// `source` = 入力手段(音声なら ready で自動生成・#160)、`timing` = 録音停止・文字起こし完了の時刻(区間ログ用)。
    func submitUtterance(_ text: String, source: ClarifyFlow.InputSource = .text, timing: ChatTiming = ChatTiming()) async {
        let text = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !text.isEmpty, morphJob == nil, !chatPending else { return }
        if routesToChat(text) {
            chatTiming = timing
            await say(text, source: source)
        } else {
            await morph(transcript: text)
        }
    }

    func routesToChat(_ text: String) -> Bool {
        guard dsl.isVoiceOnly, clarifyEnabled else { return false }
        if isChatting { return true } // 対話中の返答(「お任せ」等)は意図判定しない
        if case .morph = VoiceIntent.detect(text) { return true }
        return false
    }

    /// ユーザー発話を 1 つ足して ai に聞く。応答(question / ready)を返す。失敗時は状態カード(origin = chat)を出して nil。
    @discardableResult
    func say(_ text: String, source: ClarifyFlow.InputSource = .text) async -> ChatTurn? {
        cancelAutoBuild()
        chatLastInput = source
        withAnimation(.spring(duration: 0.4, bounce: 0.15)) {
            chatBubbles.append(ChatBubble(role: .user, text: text))
        }
        // 新規会話でも DELETE は待たない(古い会話への追記は応答の turn で検出してやり直す・#160 `ClarifyFlow.startConversation`)
        return await chatRound()
    }

    /// 今の履歴を送り直す(状態カードの「もう一度」)。
    func retryChat() {
        guard isChatting, !chatPending else { return }
        chatTask = Task { await chatRound() }
    }

    /// ready の内容で生成する(「この内容で作る」)。transcript が無ければこれまでの発話をつないで渡す。
    func buildFromChat() async {
        cancelAutoBuild()
        let transcript = chatTurn?.transcript?.trimmingCharacters(in: .whitespacesAndNewlines)
        let text = (transcript?.isEmpty == false ? transcript : nil) ?? fallbackTranscript
        guard !text.isEmpty else { return }
        print("[Metamon] chat: build transcript=\(text)")
        resetChat()
        await generate(text)
    }

    /// 「このまま作る」(質問中でも即生成)。go-ahead 語を送り backend の ready(shortcut)を待ってそのまま生成する。
    func buildNow() async {
        guard isChatting, morphJob == nil, !chatPending else { return }
        if chatTurn?.kind != .ready {
            let turn = await say(ClarifyCopy.buildNowUtterance)
            if turn == nil {
                // 対話側が落ちていても、生成はこれまでの発話で進める(作れないよりまし)
                withAnimation(.easeInOut(duration: 0.3)) { if errorCard?.origin == .chat { errorCard = nil } }
            }
        }
        await buildFromChat()
    }

    /// 会話を捨てる(「最初から」・別アプリへの切替・生成開始)。サーバー側も `DELETE /v1/chat/{appId}`。
    func resetChat() {
        cancelAutoBuild()
        chatTask?.cancel()
        chatTask = nil
        chatLastInput = nil
        chatEpoch += 1
        let hadConversation = isChatting
        let appId = document.appId
        withAnimation(.easeInOut(duration: 0.3)) {
            chatBubbles = []
            chatTurn = nil
            chatPending = false
            if errorCard?.origin == .chat { errorCard = nil }
        }
        if hadConversation {
            Task { [services] in await services.ai.resetChat(appId: appId) }
        }
    }

    // MARK: - Private

    /// 生成に渡す依頼文の予備(ready が無い時): go-ahead 語を除いたユーザー発話をつなぐ。
    private var fallbackTranscript: String {
        chatBubbles.filter { $0.role == .user && $0.text != ClarifyCopy.buildNowUtterance }.map(\.text).joined(separator: "。")
    }

    private func chatRound() async -> ChatTurn? {
        let epoch = chatEpoch
        let appId = document.appId
        let history = chatBubbles.map { ChatMessage(role: $0.role == .user ? .user : .assistant, text: $0.text) }
        var timing = chatTiming
        chatTiming = ChatTiming()
        withAnimation(.easeInOut(duration: 0.25)) {
            chatPending = true
            if errorCard?.origin == .chat { errorCard = nil }
        }
        do {
            timing.postStarted = .now
            let turn: ChatTurn
            if history.count == 1, let first = history.first {
                let started = try await ClarifyFlow.startConversation(ai: services.ai, appId: appId, first: first, locale: AppRuntime.generationLocale)
                if started.retried { print("[Metamon] chat: stale conversation on server → cleared and resent") }
                turn = started.turn
            } else {
                turn = try await services.ai.chat(appId: appId, messages: history, locale: AppRuntime.generationLocale)
            }
            timing.responded = .now
            guard epoch == chatEpoch else { return nil } // 待っている間に「最初から」/ 切替された
            print("[Metamon] chat: turn=\(turn.turn) kind=\(turn.kind.rawValue) source=\(turn.source ?? "-") text=\(turn.text)")
            withAnimation(.spring(duration: 0.4, bounce: 0.15)) {
                chatPending = false
                chatTurn = turn
                chatBubbles.append(ChatBubble(role: .assistant, text: turn.text))
            }
            await Task.yield() // 吹き出しの描画が始まるまで
            timing.shown = .now
            #if DEBUG
            print("[Metamon] chat timing: turn=\(turn.turn) kind=\(turn.kind.rawValue) \(timing.summary)")
            #endif
            SpeechReader.shared.speak(turn.text) // #43 AI の質問(録音中・OFF なら喋らない)
            scheduleAutoBuildIfNeeded(turn)
            return turn
        } catch {
            guard epoch == chatEpoch else { return nil }
            let failure = FailureKind.classify(error)
            print("[Metamon] chat failed: \(failure) (\(error))")
            withAnimation(.easeInOut(duration: 0.3)) {
                chatPending = false
                errorCard = ErrorPresentation.from(failure, transcript: history.last?.text ?? "", origin: .chat)
            }
            if failure == .sessionExpired { onSessionExpired?() }
            return nil
        }
    }

    /// 音声だけで進めていて ready が来たら、要約カードを 1.5 秒見せてから生成へ(#160)。
    /// その間に「最初から」・文字入力・ボタンタップがあれば取り消す(epoch / cancel)。
    private func scheduleAutoBuildIfNeeded(_ turn: ChatTurn) {
        guard ClarifyFlow.shouldAutoBuild(turn: turn, lastInput: chatLastInput, enabled: clarifyAutoBuild) else { return }
        let epoch = chatEpoch
        chatAutoBuildPending = true
        print("[Metamon] chat: ready via voice → auto build in 1.5s")
        chatAutoBuildTask = Task { [weak self] in
            try? await Task.sleep(for: ClarifyFlow.autoBuildDelay)
            guard let self, !Task.isCancelled, epoch == self.chatEpoch, self.chatAutoBuildPending, self.morphJob == nil else { return }
            self.chatAutoBuildPending = false
            self.chatAutoBuildTask = nil
            await self.buildFromChat()
        }
    }

    private func cancelAutoBuild() {
        chatAutoBuildTask?.cancel()
        chatAutoBuildTask = nil
        chatAutoBuildPending = false
    }

    // MARK: - DEBUG(ヘッドレス E2E)

    /// `METAMON_AUTO_TRANSCRIPT` の続き。対話に入ったら `METAMON_AUTO_REPLIES`(| 区切り)で質問に答え、尽きたら「このまま作る」。
    /// ready が来たら `METAMON_AUTO_PAUSE` 秒(既定 2.5・スクショ用)おいて「この内容で作る」。生成完了まで await する。
    func autoDrive(firstUtterance text: String) async {
        guard routesToChat(text) else {
            await morph(transcript: text)
            return
        }
        let env = ProcessInfo.processInfo.environment
        var replies = (env["METAMON_AUTO_REPLIES"] ?? "").split(separator: "|").map(String.init).filter { !$0.isEmpty }
        let pause = Double(env["METAMON_AUTO_PAUSE"] ?? "") ?? 2.5
        // `METAMON_AUTO_SOURCE=voice` で音声入力として扱う(ready の自動生成・#160 の確認)
        let source: ClarifyFlow.InputSource = env["METAMON_AUTO_SOURCE"] == "voice" ? .voice : .text
        var turn = await say(text, source: source)
        var rounds = 0
        while let current = turn, current.kind == .question, rounds < 4 {
            rounds += 1
            try? await Task.sleep(for: .seconds(pause))
            guard !replies.isEmpty else {
                print("[Metamon] E2E: chat replies exhausted → build now")
                await buildNow()
                return
            }
            turn = await say(replies.removeFirst(), source: source)
        }
        guard turn != nil else {
            print("[Metamon] E2E: chat stopped (error card shown)")
            return
        }
        if let auto = chatAutoBuildTask {
            print("[Metamon] E2E: chat ready → waiting auto build")
            await auto.value
            return
        }
        try? await Task.sleep(for: .seconds(pause))
        print("[Metamon] E2E: chat ready → build")
        await buildFromChat()
    }
}

/// 対話まわりの文言(`Greeting` / `ErrorCopy` と同じく 1 か所に)。
enum ClarifyCopy {
    /// 「このまま作る」で送る go-ahead 語(backend `buildNowPhrases` の「このまま作って」に一致)
    static var buildNowUtterance: String { L10n.tr("このまま作って") }
    static var buildNowButton: String { L10n.tr("このまま作る") }
    static var buildButton: String { L10n.tr("この内容で作る") }
    static var restart: String { L10n.tr("最初から") }
    static var thinking: String { L10n.tr("考えています…") }
    static var readyTitle: String { L10n.tr("この内容で作ります") }
    static var replyPlaceholder: String { L10n.tr("答えを文字で入力") }
    static var settingTitle: String { L10n.tr("作る前に質問する") }
    static var autoBuildSettingTitle: String { L10n.tr("答えがそろったら自動で作る") }
    static var autoBuildSettingFooter: String { L10n.tr("声だけで答えている時は、内容がそろうと少し待ってから自動で作り始めます。文字やチップで答えた時はボタンで作ります。") }
    static var autoBuilding: String { L10n.tr("まもなく作ります…") }
    static var settingFooter: String { L10n.tr("ON にすると、新しいアプリを作る前に AI が 1〜3 個だけ質問します。「このまま作る」でいつでも飛ばせます。作ったアプリの変更は質問なしですぐ反映します。") }
}
