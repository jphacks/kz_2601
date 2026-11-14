import AVFoundation
import Foundation

/// 読み上げの声と速さ(#2229・親 #1829)。声を決める所はここ 1 つ。本体の読み上げ(`SpeechReader`)・
/// `speakButton`(`LeafSpeech`)・道案内(`RouteGuideSession`)の 3 か所が `SpeechVoice.configure` を通す。
/// 設定はアプリ本体だけ(DSL・AI の決まりは変えない)。選んだ声が OS の更新などで消えたら自動に戻す。

/// 選べる声 1 つ(`AVSpeechSynthesisVoice` を値にしたもの・単体テストの対象)。
public struct SpeechVoiceOption: Sendable, Equatable, Identifiable {
    public var id: String
    public var name: String
    /// BCP 47(`ja-JP` など)
    public var language: String
    /// 0 = 既定、1 = 拡張、2 = プレミアム(`AVSpeechSynthesisVoiceQuality` の順)
    public var quality: Int
    public var isPersonal: Bool

    public init(id: String, name: String, language: String, quality: Int, isPersonal: Bool = false) {
        self.id = id
        self.name = name
        self.language = language
        self.quality = quality
        self.isPersonal = isPersonal
    }
}

/// 読み上げの速さ。既定は OS の標準の速さ。
public enum SpeechRateChoice: String, CaseIterable, Sendable, Identifiable {
    case slow
    case normal
    case fast

    public var id: String { rawValue }

    /// `AVSpeechUtterance.rate` の値(OS の最小・最大に収める)。
    public var rate: Float {
        let base = AVSpeechUtteranceDefaultSpeechRate
        let value: Float = switch self {
        case .slow: base * 0.8
        case .normal: base
        case .fast: base * 1.2
        }
        return min(max(value, AVSpeechUtteranceMinimumSpeechRate), AVSpeechUtteranceMaximumSpeechRate)
    }
}

/// 声を選ぶ規則(純粋な関数)。
public enum SpeechVoiceChoice {
    /// 選んだ結果。`voiceId` が nil なら言語だけで決める(`AVSpeechSynthesisVoice(language:)`)。
    public struct Resolution: Equatable, Sendable {
        public var voiceId: String?
        /// 選んでいた声がもう無い(自動に戻すべき)
        public var chosenIsGone: Bool
    }

    /// `language` の読み上げに使う声。
    /// - 選んだ声があり、同じ言語ならそれ
    /// - 選んだ声が消えていたら自動(`chosenIsGone`)
    /// - 選んだ声の言語が違う(英語の文を日本語の声で読まない)なら、この回だけ自動
    /// - 自動 = その言語で品質の一番高い声(パーソナルボイスは選ばない・同じ品質なら先のもの)
    public static func resolve(language: String, chosenId: String?, options: [SpeechVoiceOption]) -> Resolution {
        if let chosenId {
            if let chosen = options.first(where: { $0.id == chosenId }) {
                if chosen.language == language { return Resolution(voiceId: chosen.id, chosenIsGone: false) }
            } else {
                return Resolution(voiceId: automatic(language: language, options: options), chosenIsGone: true)
            }
        }
        return Resolution(voiceId: automatic(language: language, options: options), chosenIsGone: false)
    }

    static func automatic(language: String, options: [SpeechVoiceOption]) -> String? {
        var best: SpeechVoiceOption?
        for o in options where o.language == language && !o.isPersonal {
            if best == nil || o.quality > best!.quality { best = o }
        }
        return best?.id
    }

    /// 設定に出す一覧: その言語の声を、パーソナルボイス → 品質の高い順 → 名前順。
    public static func options(for language: String, in all: [SpeechVoiceOption]) -> [SpeechVoiceOption] {
        all.filter { $0.language == language }.sorted { a, b in
            if a.isPersonal != b.isPersonal { return a.isPersonal }
            if a.quality != b.quality { return a.quality > b.quality }
            return a.name.localizedStandardCompare(b.name) == .orderedAscending
        }
    }
}

/// 選んだ声・速さの保存(UserDefaults・言語ごと)。
public struct SpeechVoicePreference {
    public var defaults: UserDefaults

    public init(defaults: UserDefaults = .standard) {
        self.defaults = defaults
    }

    static func voiceKey(_ language: String) -> String { "metamon.speech.voice.\(language)" }
    static let rateKey = "metamon.speech.rate"

    /// nil = 自動
    public func voiceId(for language: String) -> String? {
        defaults.string(forKey: Self.voiceKey(language))
    }

    public func setVoiceId(_ id: String?, for language: String) {
        if let id { defaults.set(id, forKey: Self.voiceKey(language)) } else { defaults.removeObject(forKey: Self.voiceKey(language)) }
    }

    public var rate: SpeechRateChoice {
        get { defaults.string(forKey: Self.rateKey).flatMap(SpeechRateChoice.init(rawValue:)) ?? .normal }
        nonmutating set { defaults.set(newValue.rawValue, forKey: Self.rateKey) }
    }
}

/// 読み上げの声を決める唯一の所。
@MainActor
public enum SpeechVoice {
    /// 設定で選べる言語(読み上げで使う 2 つ)
    public static let languages = ["ja-JP", "en-US"]

    public static var preference = SpeechVoicePreference()

    private static var cachedOptions: [SpeechVoiceOption]?
    private static var cachedVoices: [String: AVSpeechSynthesisVoice] = [:]

    /// 端末にある声(パーソナルボイスは許可がある時だけ OS が返す)。声が増減したら `reload()`。
    public static var options: [SpeechVoiceOption] {
        if let cachedOptions { return cachedOptions }
        let voices = AVSpeechSynthesisVoice.speechVoices()
        cachedVoices = Dictionary(voices.map { ($0.identifier, $0) }, uniquingKeysWith: { a, _ in a })
        let mapped = voices.map(option)
        cachedOptions = mapped
        return mapped
    }

    /// 声の一覧を読み直す(設定を開いた時・パーソナルボイスを許可した時)。
    public static func reload() {
        cachedOptions = nil
        cachedVoices = [:]
    }

    /// `language` で読む時の声。選んだ声が消えていたら保存も自動に戻す。
    public static func voice(for language: String) -> AVSpeechSynthesisVoice? {
        let resolution = SpeechVoiceChoice.resolve(language: language, chosenId: preference.voiceId(for: language), options: options)
        if resolution.chosenIsGone { preference.setVoiceId(nil, for: language) }
        if let id = resolution.voiceId, let voice = cachedVoices[id] ?? AVSpeechSynthesisVoice(identifier: id) { return voice }
        return AVSpeechSynthesisVoice(language: language)
    }

    /// 発話に声と速さを入れる。3 か所の読み上げはここを通す。
    public static func configure(_ utterance: AVSpeechUtterance, language: String) {
        utterance.voice = voice(for: language)
        utterance.rate = preference.rate.rate
    }

    /// パーソナルボイスを使う許可(未決定の時だけ OS に聞く)。
    public static var personalVoiceStatus: AVSpeechSynthesizer.PersonalVoiceAuthorizationStatus {
        AVSpeechSynthesizer.personalVoiceAuthorizationStatus
    }

    public static func requestPersonalVoice() async -> AVSpeechSynthesizer.PersonalVoiceAuthorizationStatus {
        let status = await withCheckedContinuation { cont in
            AVSpeechSynthesizer.requestPersonalVoiceAuthorization { cont.resume(returning: $0) }
        }
        reload()
        return status
    }

    static func option(_ v: AVSpeechSynthesisVoice) -> SpeechVoiceOption {
        let quality: Int = switch v.quality {
        case .premium: 2
        case .enhanced: 1
        default: 0
        }
        return SpeechVoiceOption(id: v.identifier, name: v.name, language: v.language, quality: quality,
                                 isPersonal: v.voiceTraits.contains(.isPersonalVoice))
    }
}
