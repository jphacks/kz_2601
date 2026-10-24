import SwiftUI
import MetamonServices

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
}

/// 生成前の対話(#75 / backend #74 `POST /v1/chat`)。
/// 初期(音声)画面で、最初の発話を即生成せず ai に聞き返させ、1〜3 ラリーで要件を固めてから `/v1/generate` に渡す。
/// - 会話状態は ai が appId ごとに持つ。iOS は手元の全履歴を毎回送る(2 件以上はサーバー側を置き換える契約)
/// - ready の `transcript` を `generate(_:)`(意図判定なし)に渡す。generate の契約は無変更
/// - 「このまま作る」は go-ahead 語を送って backend の shortcut ready に任せる(失敗してもこれまでの発話で生成する)
/// - edit(生成後の改変)は対話を挟まない
extension AppRuntime {
    /// 対話中か(吹き出しが 1 つ以上)。
    var isChatting: Bool { !chatBubbles.isEmpty }

    /// 音声画面からの発話の入口。初期画面 + 対話 ON + 通常の依頼(「元に戻して」等でない)なら対話へ、それ以外は従来どおり即生成。
    func submitUtterance(_ text: String) async {
        let text = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !text.isEmpty, morphJob == nil, !chatPending else { return }
        if routesToChat(text) {
            await say(text)
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
    func say(_ text: String) async -> ChatTurn? {
        let isFirst = chatBubbles.isEmpty
        withAnimation(.spring(duration: 0.4, bounce: 0.15)) {
            chatBubbles.append(ChatBubble(role: .user, text: text))
        }
        if isFirst {
            // 新規会話: 前の会話(30 分以内に同じ appId で始めたもの)に追記されないよう先に消す
            await services.ai.resetChat(appId: document.appId)
        }
        return await chatRound()
    }

    /// 今の履歴を送り直す(状態カードの「もう一度」)。
    func retryChat() {
        guard isChatting, !chatPending else { return }
        chatTask = Task { await chatRound() }
    }

    /// ready の内容で生成する(「この内容で作る」)。transcript が無ければこれまでの発話をつないで渡す。
    func buildFromChat() async {
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
        chatTask?.cancel()
        chatTask = nil
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
        withAnimation(.easeInOut(duration: 0.25)) {
            chatPending = true
            if errorCard?.origin == .chat { errorCard = nil }
        }
        do {
            let turn = try await services.ai.chat(appId: appId, messages: history, locale: nil)
            guard epoch == chatEpoch else { return nil } // 待っている間に「最初から」/ 切替された
            print("[Metamon] chat: turn=\(turn.turn) kind=\(turn.kind.rawValue) source=\(turn.source ?? "-") text=\(turn.text)")
            withAnimation(.spring(duration: 0.4, bounce: 0.15)) {
                chatPending = false
                chatTurn = turn
                chatBubbles.append(ChatBubble(role: .assistant, text: turn.text))
            }
            SpeechReader.shared.speak(turn.text) // #43 AI の質問(録音中・OFF なら喋らない)
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
        var turn = await say(text)
        var rounds = 0
        while let current = turn, current.kind == .question, rounds < 4 {
            rounds += 1
            try? await Task.sleep(for: .seconds(pause))
            guard !replies.isEmpty else {
                print("[Metamon] E2E: chat replies exhausted → build now")
                await buildNow()
                return
            }
            turn = await say(replies.removeFirst())
        }
        guard turn != nil else {
            print("[Metamon] E2E: chat stopped (error card shown)")
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
    static let buildNowUtterance = "このまま作って"
    static let buildNowButton = "このまま作る"
    static let buildButton = "この内容で作る"
    static let restart = "最初から"
    static let thinking = "考えています…"
    static let readyTitle = "この内容で作ります"
    static let replyPlaceholder = "答えを文字で入力"
    static let settingTitle = "作る前に質問する"
    static let settingFooter = "ON にすると、新しいアプリを作る前に AI が 1〜3 個だけ質問します。「このまま作る」でいつでも飛ばせます。作ったアプリの変更は質問なしですぐ反映します。"
}
