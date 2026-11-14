import Foundation
import MetamonServices
import MetamonRenderer

/// 生成したアプリの `fileImport` accept audio(録音ファイルを文字にする)。音声入力と同じ `services.transcriber` の本命(WhisperKit・端末内)だけを使う。
/// 音声も文字もサーバー・AI に送らない(仮の聞き取りの Apple の認識は短い発話向けなので使わない。サーバーで認識する経路は元から無い)。
extension AppRuntime: AudioFileTranscriptionHost {
    private var fileTranscriber: (any AudioFileTranscriber)? { services.transcriber as? any AudioFileTranscriber }

    var audioFileReadiness: DictationReadiness {
        guard let t = fileTranscriber else { return .failed }
        if t.canTranscribeFiles { return .ready }
        switch services.transcriber.status {
        case .notDownloaded: return .needsModel
        case .failed: return .failed
        case .ready, .downloading, .loading, .recognizing: return .preparing // ready でも録音・別の文字起こし中
        }
    }

    func prepareAudioFileTranscription() { prepareDictation() }

    func transcribeAudioFile(_ url: URL, progress: @escaping @MainActor (Double) -> Void) async throws -> String {
        guard let t = fileTranscriber, !showVoiceSheet else {
            throw DictationFailure(L10n.tr("音声入力を使っている間は、録音ファイルを文字にできません"))
        }
        do {
            return try await t.transcribeFile(at: url, progress: progress)
        } catch is CancellationError {
            throw CancellationError()
        } catch let f as TranscriberFailure {
            throw DictationFailure(ErrorCopy.transcriberFailure(f))
        } catch {
            if Task.isCancelled { throw CancellationError() }
            throw DictationFailure(ErrorCopy.failure(L10n.tr("録音ファイルを文字にできませんでした"), error, context: "audioFileImport"))
        }
    }
}
