import AVFoundation
import Observation

/// `speakButton` の読み上げ(アプリ全体で 1 本)。声は UI の言語(`LanguagePreference`)。
///
/// 音声セッション: `usesApplicationAudioSession = false` でシステムの別セッションで喋り、アプリの共有セッションに触らない
/// (読み上げ #43 の `SpeechReader` と同じ)。録音(音声入力・`transcribeField`)の category を壊さないため。
/// 録音の間は `stopForRecording()` で止めて新しい読み上げを抑止する(TTS の音をマイクが拾わない)。
@MainActor @Observable
public final class LeafSpeech: NSObject, AVSpeechSynthesizerDelegate {
    public static let shared = LeafSpeech()

    /// 読み上げ中の部品(`speakButton` の id)。nil = 読んでいない。
    public private(set) var speakingOwner: String?
    @ObservationIgnored private let synthesizer = AVSpeechSynthesizer()
    @ObservationIgnored private var micBusy = false

    override private init() {
        super.init()
        #if os(iOS)
        synthesizer.usesApplicationAudioSession = false
        #endif
        synthesizer.delegate = self
    }

    /// UI の言語の声(ja → ja-JP、en → en-US)。
    nonisolated static func voiceLanguage(uiCode: String) -> String {
        uiCode == "en" ? "en-US" : "ja-JP"
    }

    /// 読み上げる。前の読み上げは打ち切る。録音中・空文字なら false。
    @discardableResult
    public func speak(_ text: String, owner: String) -> Bool {
        let text = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !text.isEmpty, !micBusy else { return false }
        if synthesizer.isSpeaking { synthesizer.stopSpeaking(at: .immediate) }
        let utterance = AVSpeechUtterance(string: text)
        utterance.voice = AVSpeechSynthesisVoice(language: Self.voiceLanguage(uiCode: LanguagePreference.shared.code))
        speakingOwner = owner
        synthesizer.speak(utterance)
        return true
    }

    /// 止める。`owner` を渡したら、その部品が読んでいる時だけ。
    public func stop(owner: String? = nil) {
        if let owner, owner != speakingOwner { return }
        speakingOwner = nil
        if synthesizer.isSpeaking { synthesizer.stopSpeaking(at: .immediate) }
    }

    /// 録音を始める時: 止めて、`recordingFinished()` まで読み上げない。
    public func stopForRecording() {
        micBusy = true
        stop()
    }

    public func recordingFinished() {
        micBusy = false
    }

    nonisolated public func speechSynthesizer(_ synthesizer: AVSpeechSynthesizer, didFinish utterance: AVSpeechUtterance) {
        Task { @MainActor in self.finished() }
    }

    nonisolated public func speechSynthesizer(_ synthesizer: AVSpeechSynthesizer, didCancel utterance: AVSpeechUtterance) {
        Task { @MainActor in self.finished() }
    }

    private func finished() {
        if !synthesizer.isSpeaking { speakingOwner = nil }
    }
}
