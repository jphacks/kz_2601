import Foundation
import MetamonServices
import MetamonRenderer

/// 生成したアプリの話して入力(`DictationHost`・aiChat のマイク・transcribeField)を `SpeechTranscriber` でつなぐ(#1328)。
/// 声は端末の外に出さない。認識(`transcriber`)・読み上げの止め方・失敗の文言・モデルの用意はホストが差し替える。
/// マイクは 1 つなので、部品が録音を始めた時だけ `isRecording` を立て、捨てる時もその録音だけを捨てる
/// (声で頼む・音声入力の録音を部品の「画面を離れた」で止めない)。
@MainActor
final class TranscriberDictation {
    let transcriber: any SpeechTranscriber
    /// 声で頼む・音声入力を出している間 true(部品は録音中ならやめる)
    private let voiceInputActive: () -> Bool
    /// 録音を始める前に読み上げを止め、終わったら戻す
    private let stopSpeech: () -> Void
    private let resumeSpeech: () -> Void
    /// 失敗の文言(許可が無い時は設定の開き先も)
    private let failure: (Error) -> DictationFailure
    /// モデルが無い時: 用意を始める(ダウンロードの確認はホストが出す)
    private let requestModel: () -> Void

    /// 部品が録音しているか(始めてから止める・捨てるまで)
    private(set) var isRecording = false

    init(transcriber: any SpeechTranscriber,
         voiceInputActive: @escaping () -> Bool,
         stopSpeech: @escaping () -> Void,
         resumeSpeech: @escaping () -> Void,
         failure: @escaping (Error) -> DictationFailure,
         requestModel: @escaping () -> Void) {
        self.transcriber = transcriber
        self.voiceInputActive = voiceInputActive
        self.stopSpeech = stopSpeech
        self.resumeSpeech = resumeSpeech
        self.failure = failure
        self.requestModel = requestModel
    }

    var readiness: DictationReadiness {
        Self.readiness(isReady: transcriber.isReady, status: transcriber.status)
    }

    var isVoiceInputActive: Bool { voiceInputActive() }

    /// 本命のモデルが文字にしている間は、仮の聞き取りが使えても録音を始めない(iOS と同じ)。
    nonisolated static func readiness(isReady: Bool, status: TranscriberStatus) -> DictationReadiness {
        if status == .recognizing { return .preparing }
        if isReady { return .ready }
        switch status {
        case .ready: return .ready
        case .downloading, .loading, .recognizing: return .preparing
        case .failed: return .failed
        case .notDownloaded: return .needsModel
        }
    }

    /// 失敗の後は作り直す。モデルが無ければホストの入口(ダウンロードの確認)へ。
    func prepare() {
        if case .failed = transcriber.status {
            Task { try? await transcriber.prepare() }
        } else {
            requestModel()
        }
    }

    func start() async throws {
        stopSpeech()
        do {
            try await transcriber.startRecording()
            isRecording = true
        } catch {
            resumeSpeech()
            throw failure(error)
        }
    }

    func finish() async throws -> String {
        isRecording = false
        resumeSpeech()
        let text: String
        do {
            text = try await transcriber.stopAndTranscribe()
        } catch {
            throw failure(error)
        }
        guard !text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
            throw DictationFailure(L10n.tr("音声を聞き取れませんでした。もう一度はっきり話してください"))
        }
        return text
    }

    /// 部品の録音だけを捨てる。録音していなければ何もしない(ほかの録音を止めない)。
    func cancel() {
        guard isRecording else { return }
        isRecording = false
        transcriber.cancelRecording()
        resumeSpeech()
    }
}
