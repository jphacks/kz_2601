import AppKit
import AVFoundation
import Observation
import MetamonRenderer

/// 声を出す部分。テストで差し替える
@MainActor
protocol MacSpeechEngine: AnyObject {
    var isSpeaking: Bool { get }
    func speak(_ text: String, language: String)
    func stop()
}

/// 完成の要約を Mac の声で読み上げる(#1210)。iOS の `SpeechReader`(#43)と同じ保存先(`metamon.speech.enabled`)・既定 OFF。
/// VoiceOver が動いている時は喋らない(要約のお知らせが VoiceOver に伝える・重ねない)。
/// 新しい読み上げ・会話を始めた時・録音を始めた時・ウインドウを閉じた時に止める。
@MainActor @Observable
final class MacSpeechReader {
    static let shared = MacSpeechReader()
    static let key = "metamon.speech.enabled"

    var enabled: Bool {
        didSet {
            defaults.set(enabled, forKey: Self.key)
            if !enabled { stop() }
        }
    }

    @ObservationIgnored private let engine: MacSpeechEngine
    @ObservationIgnored private let defaults: UserDefaults
    @ObservationIgnored private let isVoiceOverRunning: @MainActor () -> Bool
    @ObservationIgnored private let languageCode: @MainActor () -> String

    init(engine: MacSpeechEngine? = nil,
         defaults: UserDefaults = .standard,
         isVoiceOverRunning: @escaping @MainActor () -> Bool = { NSWorkspace.shared.isVoiceOverEnabled },
         languageCode: @escaping @MainActor () -> String = { LanguagePreference.shared.code }) {
        self.engine = engine ?? SystemEngine()
        self.defaults = defaults
        self.isVoiceOverRunning = isVoiceOverRunning
        self.languageCode = languageCode
        enabled = Self.storedEnabled(defaults)
    }

    /// 読み上げの言語(アプリの表示言語・#44)
    nonisolated static func voiceLanguage(for code: String) -> String { code == "en" ? "en-US" : "ja-JP" }

    /// 要約を読み上げる。前の読み上げは打ち切る。OFF・VoiceOver の時・空文字なら喋らない
    func speak(_ text: String) {
        let text = text.trimmingCharacters(in: .whitespacesAndNewlines)
        stop()
        guard enabled, !text.isEmpty, !isVoiceOverRunning() else { return }
        engine.speak(text, language: Self.voiceLanguage(for: languageCode()))
    }

    func stop() {
        guard engine.isSpeaking else { return }
        engine.stop()
    }

    private static func storedEnabled(_ defaults: UserDefaults) -> Bool {
        switch ProcessInfo.processInfo.environment["METAMON_SPEECH"] {
        case "0": false
        case "1": true
        default: defaults.object(forKey: key) as? Bool ?? false
        }
    }

    /// AVSpeechSynthesizer で喋る。言語ごとに一番質の高い声を選ぶ(iOS と同じ)
    final class SystemEngine: MacSpeechEngine {
        private let synthesizer = AVSpeechSynthesizer()
        private var voices: [String: AVSpeechSynthesisVoice] = [:]

        var isSpeaking: Bool { synthesizer.isSpeaking }

        func speak(_ text: String, language: String) {
            let utterance = AVSpeechUtterance(string: text)
            utterance.voice = voice(for: language)
            utterance.rate = AVSpeechUtteranceDefaultSpeechRate
            synthesizer.speak(utterance)
        }

        func stop() { synthesizer.stopSpeaking(at: .immediate) }

        private func voice(for language: String) -> AVSpeechSynthesisVoice? {
            if let cached = voices[language] { return cached }
            let best = AVSpeechSynthesisVoice.speechVoices()
                .filter { $0.language == language }
                .max { $0.quality.rawValue < $1.quality.rawValue }
                ?? AVSpeechSynthesisVoice(language: language)
            voices[language] = best
            return best
        }
    }
}
