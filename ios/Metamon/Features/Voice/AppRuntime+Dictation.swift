import Foundation
import MetamonServices
import MetamonRenderer

/// 生成したアプリの `transcribeField`(話して入力)。マイク FAB の音声入力と同じ `services.transcriber`(WhisperKit・端末内)を使う。
/// 声は端末の外に出さない。音声入力のシートが出ている間は部品が録音を捨てるので、マイクを取り合わない。
extension AppRuntime: DictationHost {
    var dictationReadiness: DictationReadiness {
        Self.dictationReadiness(isReady: services.transcriber.isReady, status: services.transcriber.status)
    }

    /// 本命のモデルが文字にしている間(録音ファイルの文字起こしなど)は、仮の聞き取りが使えても録音を始めない(モデルを使っている間)。
    /// 部品の自分の録音を文字にしている間も recognizing だが、その間の部品は「文字にしています」を出していて読まない。
    nonisolated static func dictationReadiness(isReady: Bool, status: TranscriberStatus) -> DictationReadiness {
        if status == .recognizing { return .preparing }
        if isReady { return .ready }
        switch status {
        case .ready: return .ready
        case .downloading, .loading, .recognizing: return .preparing
        case .failed: return .failed
        case .notDownloaded: return .needsModel
        }
    }

    var isVoiceInputActive: Bool { showVoiceSheet }

    /// 起動時と同じ入口(モデルが無ければサイズを示して確認・#246)。失敗後は作り直す。
    func prepareDictation() {
        if case .failed = services.transcriber.status {
            Task { try? await services.transcriber.prepare() }
        } else {
            prepareTranscriberIfAllowed()
        }
    }

    private static func dictationMessage(_ error: Error) -> String {
        if let f = error as? TranscriberFailure { return ErrorCopy.transcriberFailure(f) }
        return ErrorCopy.failure(L10n.tr("録音できませんでした"), error, context: "dictation")
    }

    func startDictation() async throws {
        await SpeechReader.shared.stopForRecording() // #43 と speakButton の読み上げを止めてから録音
        do {
            try await services.transcriber.startRecording()
        } catch {
            SpeechReader.shared.recordingFinished()
            throw DictationFailure(Self.dictationMessage(error))
        }
    }

    func finishDictation() async throws -> String {
        SpeechReader.shared.recordingFinished()
        let text: String
        do {
            text = try await services.transcriber.stopAndTranscribe()
        } catch {
            throw DictationFailure(Self.dictationMessage(error))
        }
        guard !text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
            throw DictationFailure(L10n.tr("音声を聞き取れませんでした。もう一度はっきり話してください"))
        }
        return text
    }

    func cancelDictation() {
        services.transcriber.cancelRecording()
        SpeechReader.shared.recordingFinished()
    }
}
