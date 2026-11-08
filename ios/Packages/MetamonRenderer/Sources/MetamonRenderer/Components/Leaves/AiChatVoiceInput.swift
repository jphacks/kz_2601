#if !APP_STORE_REVIEW
import Foundation

/// aiChat の入力欄のマイク(話して入力)。`transcribeField` と同じ `DictationHost`(端末内の音声モデル)で入力欄に文字を足す。
/// 送るのはユーザー(自動で送らない)。録音中は読み上げない・送らない。
enum AiChatMic: Equatable {
    /// ホストが話して入力を持たない(プレビュー・テスト)
    case hidden
    /// 押すと録音を始める
    case start
    /// 録音を始めている(押せない・二重に始めない)
    case starting
    /// 録音中。押すと止めて文字にする
    case stop
    /// 文字にしている
    case transcribing
    /// 押せない(モデルの準備中・録音ファイルの文字起こし中・マイク FAB の音声入力中・AI と話せない版)
    case busy
    /// モデルが無い・失敗した。押すと用意を始める
    case prepare

    static func state(phase: DictationPhase, readiness: DictationReadiness?, voiceInputActive: Bool, online: Bool) -> AiChatMic {
        guard let readiness else { return .hidden }
        switch phase {
        case .starting: return .starting
        case .listening: return .stop
        case .transcribing: return .transcribing
        case .idle: break
        }
        guard online, !voiceInputActive else { return .busy }
        switch readiness {
        case .ready: return .start
        case .preparing: return .busy
        case .needsModel, .failed: return .prepare
        }
    }

    var enabled: Bool { self == .start || self == .stop || self == .prepare }

    /// 録音・文字起こしの間は送らない(聞き取った文が入力欄に入る前に送ってしまう)。
    static func canSend(online: Bool, sending: Bool, draft: String, phase: DictationPhase) -> Bool {
        online && !sending && phase == .idle && !draft.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
    }

    /// 吹き出しのスピーカー: 録音・文字起こしの間は押せない(読み上げの声をマイクが拾う)。
    static func canSpeak(phase: DictationPhase) -> Bool { phase == .idle }

    /// 録音を始めてよいか。始めている・録音中・文字起こし中は始めない(二重に始めると片方の失敗で読み上げの抑止が外れる)。
    static func canStart(phase: DictationPhase) -> Bool { phase == .idle }
}

/// 録音中に届いた返事の VoiceOver の読み上げを録音が終わるまで待たせる(最新の 1 件だけ)。
struct AiChatAnnounceQueue: Equatable {
    private(set) var pending: String?

    /// 返事が届いた。`.queue` なら持っておく(古い分は捨てる)。
    mutating func received(_ text: String, mode: AiChatLeaf.AutoRead) {
        if mode == .queue { pending = text }
    }

    /// 録音の段階が変わった。録音が終わった(idle)ら待たせていた文を返す。
    mutating func phaseChanged(to phase: DictationPhase, visible: Bool, voiceOver: Bool) -> String? {
        guard phase == .idle, let text = pending else { return nil }
        pending = nil
        return visible && voiceOver ? text : nil
    }

    mutating func clear() { pending = nil }
}
#endif
