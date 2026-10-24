import Foundation

/// 生成前の対話(#75)の、UI に依存しない判断(#160 体感速度)。AppRuntime から呼び、単体テストはモックで回す。
public enum ClarifyFlow {
    /// 最後に吹き出しを足した入力手段。自動生成の判定に使う。
    public enum InputSource: Sendable, Equatable { case voice, text, chip }

    /// 新しい会話の 1 発目を送る(#160)。`DELETE` を先に await しない。
    ///
    /// backend の契約(chat.go `mergeHistory`)は「messages が 1 件ならサーバー側の会話に追記」なので、
    /// 前の会話(30 分以内・同じ appId)が残っていると追記されてしまう。追記されたかは応答の `turn`
    /// (サーバー側のユーザー発話数)で分かる: 1 件だけ送って `turn != 1` なら古い会話に混ざっている。
    /// その時だけ `DELETE` を await して同じ 1 件を送り直す(Claude 1 回ぶん無駄になるが、まず起きない)。
    /// - 通常(前の会話は「最初から」/ 生成開始 / 切替で消し済み): POST 1 回だけ
    /// - DELETE を投げっぱなしにしない理由: POST の後に DELETE が届くと新しい会話(1 発目で作った brief)を消し、
    ///   「このまま作る」の即答(shortcut)が Claude 呼び出しに落ちて遅くなる
    public static func startConversation(ai: any AIClient, appId: String, first: ChatMessage, locale: String?) async throws -> (turn: ChatTurn, retried: Bool) {
        let turn = try await ai.chat(appId: appId, messages: [first], locale: locale)
        guard turn.turn != 1 else { return (turn, false) }
        await ai.resetChat(appId: appId)
        try Task.checkCancellation()
        return (try await ai.chat(appId: appId, messages: [first], locale: locale), true)
    }

    /// ready を受けたら確認ボタンを待たずに生成へ進むか(#160)。
    /// 音声だけで進めている(最後の入力がマイク)時だけ。文字・チップで進めた時は従来どおりボタンで。
    public static func shouldAutoBuild(turn: ChatTurn, lastInput: InputSource?, enabled: Bool) -> Bool {
        enabled && turn.kind == .ready && lastInput == .voice
    }

    /// 自動生成の前に要約カードを見せる時間。
    public static let autoBuildDelay: Duration = .milliseconds(1500)
}

/// 対話 1 ラリーの区間ごとの時刻(#160)。`[Metamon] chat timing: ...` の 1 行にする。
/// 録音停止 → 文字起こし完了 → POST 開始 → 応答受信 → 吹き出し表示。文字入力では前 2 つが nil。
public struct ChatTiming: Sendable {
    public var recordStopped: ContinuousClock.Instant?
    public var transcribed: ContinuousClock.Instant?
    public var postStarted: ContinuousClock.Instant?
    public var responded: ContinuousClock.Instant?
    public var shown: ContinuousClock.Instant?

    public init(recordStopped: ContinuousClock.Instant? = nil, transcribed: ContinuousClock.Instant? = nil) {
        self.recordStopped = recordStopped
        self.transcribed = transcribed
    }

    static func ms(_ a: ContinuousClock.Instant?, _ b: ContinuousClock.Instant?) -> String {
        guard let a, let b else { return "-" }
        let c = (b - a).components
        return "\(c.seconds * 1000 + c.attoseconds / 1_000_000_000_000_000)ms"
    }

    /// 例: `stt=1180ms pre=12ms net=2310ms render=4ms total=3506ms`(取れていない区間は `-`)
    public var summary: String {
        let start = recordStopped ?? transcribed ?? postStarted
        return "stt=\(Self.ms(recordStopped, transcribed)) pre=\(Self.ms(transcribed ?? recordStopped, postStarted)) "
            + "net=\(Self.ms(postStarted, responded)) render=\(Self.ms(responded, shown)) total=\(Self.ms(start, shown))"
    }
}
