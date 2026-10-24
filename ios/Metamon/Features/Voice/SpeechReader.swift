import AVFoundation
import Observation

/// 挨拶・生成完了の summary・生成前の対話(AI の質問)を読み上げる(#43)。
///
/// AVAudioSession の扱い(「聞き取れませんでした」の再発防止):
/// - `usesApplicationAudioSession = false` でシステム管理の別セッションで喋らせ、アプリの共有セッションには一切触らない。
///   セッションの category / active を決めるのは録音側(`WhisperKitRecognizer.startRecording` の `.playAndRecord`)だけ。
/// - マイクを押したら `stopForRecording()` で即停止し、録音が終わるまで(`recordingFinished()`)新しい読み上げを抑止する。
///   録音中に AI の応答が届いても喋らない(TTS の音をマイクが拾わない)。
@MainActor @Observable
final class SpeechReader: NSObject, AVSpeechSynthesizerDelegate {
    static let shared = SpeechReader()

    /// 既定 OFF: デモ会場の騒音で聞き取れない・操作前に勝手に喋り出す印象を避けるため。設定で ON にした人だけ読み上げる。
    /// `METAMON_SPEECH=0/1` があれば UserDefaults より優先(E2E・デモ端末用)。
    var enabled: Bool = SpeechReader.storedEnabled {
        didSet {
            UserDefaults.standard.set(enabled, forKey: Self.key)
            if !enabled { stop() }
        }
    }

    @ObservationIgnored private let synthesizer = AVSpeechSynthesizer()
    @ObservationIgnored private var micBusy = false
    @ObservationIgnored private var voices: [String: AVSpeechSynthesisVoice] = [:]

    private override init() {
        super.init()
        synthesizer.usesApplicationAudioSession = false
        synthesizer.delegate = self
    }

    /// UI 言語に合わせた読み上げ言語(英語化 #44 で en のローカライズが入れば自動で en-US になる)。
    nonisolated static var uiLanguage: String {
        (Bundle.main.preferredLocalizations.first ?? "ja").hasPrefix("en") ? "en-US" : "ja-JP"
    }

    /// 読み上げる。前の読み上げは打ち切る。OFF・録音中・空文字なら何もしない。
    func speak(_ text: String, language: String = SpeechReader.uiLanguage) {
        let text = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard enabled, !micBusy, !text.isEmpty else { return }
        if synthesizer.isSpeaking { synthesizer.stopSpeaking(at: .immediate) }
        let utterance = AVSpeechUtterance(string: text)
        utterance.voice = voice(for: language)
        utterance.rate = AVSpeechUtteranceDefaultSpeechRate
        print("[Metamon] speech: start lang=\(language) voice=\(utterance.voice?.identifier ?? "-") text=\(text)")
        synthesizer.speak(utterance)
    }

    /// 読み上げを即停止(画面遷移・設定 OFF など)。
    func stop() {
        guard synthesizer.isSpeaking else { return }
        synthesizer.stopSpeaking(at: .immediate)
        print("[Metamon] speech: stop")
    }

    /// マイクタップ時: 即停止し、録音が終わるまで読み上げを抑止する。止まり切るまで最大 0.3 秒待つ(TTS の尾をマイクが拾わないように)。
    func stopForRecording() async {
        micBusy = true
        guard synthesizer.isSpeaking else { return }
        synthesizer.stopSpeaking(at: .immediate)
        print("[Metamon] speech: stop (mic tapped)")
        for _ in 0..<6 where synthesizer.isSpeaking {
            try? await Task.sleep(for: .milliseconds(50))
        }
    }

    /// 録音終了(成功・失敗とも)。読み上げの抑止を解く。
    func recordingFinished() {
        micBusy = false
    }

    /// 指定言語の最高品質ボイス(premium > enhanced > default)。無ければ言語指定のみ。
    private func voice(for language: String) -> AVSpeechSynthesisVoice? {
        if let cached = voices[language] { return cached }
        let best = AVSpeechSynthesisVoice.speechVoices()
            .filter { $0.language == language }
            .max { $0.quality.rawValue < $1.quality.rawValue }
            ?? AVSpeechSynthesisVoice(language: language)
        voices[language] = best
        return best
    }

    // MARK: - AVSpeechSynthesizerDelegate(ログのみ)

    nonisolated func speechSynthesizer(_ synthesizer: AVSpeechSynthesizer, didFinish utterance: AVSpeechUtterance) {
        print("[Metamon] speech: finished")
    }

    nonisolated func speechSynthesizer(_ synthesizer: AVSpeechSynthesizer, didCancel utterance: AVSpeechUtterance) {
        print("[Metamon] speech: cancelled")
    }

    // MARK: - 設定

    private static let key = "metamon.speech.enabled"

    private static var storedEnabled: Bool {
        switch ProcessInfo.processInfo.environment["METAMON_SPEECH"] {
        case "0": return false
        case "1": return true
        default: return UserDefaults.standard.object(forKey: key) as? Bool ?? false
        }
    }
}

/// 読み上げまわりの文言(`ClarifyCopy` と同じく 1 か所に)。
enum SpeechCopy {
    static let sectionTitle = "読み上げ"
    static let settingTitle = "挨拶と質問を読み上げる"
    static let settingFooter = "ON にすると、最初の挨拶・AI の質問・アプリができた時の説明を声で読み上げます。マイクを押すとすぐ止まります。"
}
