import Foundation
import MetamonRenderer
import MetamonServices

/// 生成したアプリの `fileImport` accept audio(録音ファイルを文字にする・#952)。iOS の `AppRuntime+AudioFileTranscription` と同じ形。
/// 声で頼む・話して入力と同じ認識の本命(WhisperKit・Mac の中)だけを使い、音声も文字もサーバー・AI に送らない
/// (仮の聞き取りの Apple の認識は短い発話向けなので使わない)。マイクの装置の選択はファイルには関係しない。
/// 声の入力・audioRecorder が録音している間は始めない(録音と同時に 1 つだけ)。
extension MacAppRuntime: AudioFileTranscriptionHost {
    private var fileTranscriber: (any AudioFileTranscriber)? { dictationSource?.transcriber as? any AudioFileTranscriber }

    var audioFileReadiness: DictationReadiness {
        guard let t = fileTranscriber, let source = dictationSource else { return .failed }
        return Self.audioFileReadiness(canTranscribe: t.canTranscribeFiles && !audioFocus.isRecording,
                                       status: source.transcriber.status)
    }

    /// iOS と同じ: 用意済みなら ready、ready でも録音・別の文字起こし中は preparing
    nonisolated static func audioFileReadiness(canTranscribe: Bool, status: TranscriberStatus) -> DictationReadiness {
        if canTranscribe { return .ready }
        switch status {
        case .notDownloaded: return .needsModel
        case .failed: return .failed
        case .ready, .downloading, .loading, .recognizing: return .preparing
        }
    }

    func prepareAudioFileTranscription() { prepareDictation() }

    func transcribeAudioFile(_ url: URL, progress: @escaping @MainActor (Double) -> Void) async throws -> String {
        guard let t = fileTranscriber, !isVoiceInputActive, !(dictationSource?.isRecording ?? false) else {
            throw DictationFailure(L10n.tr("音声入力を使っている間は、録音ファイルを文字にできません"))
        }
        guard !audioFocus.isRecording else { throw DictationFailure(MacVoiceCopy.otherRecording) }
        do {
            return try await t.transcribeFile(at: url, progress: progress)
        } catch is CancellationError {
            throw CancellationError()
        } catch let f as TranscriberFailure {
            throw MacVoiceCopy.dictationFailure(f)
        } catch {
            if Task.isCancelled { throw CancellationError() }
            throw DictationFailure(L10n.tr("録音ファイルを文字にできませんでした。もう一度お試しください"))
        }
    }
}
