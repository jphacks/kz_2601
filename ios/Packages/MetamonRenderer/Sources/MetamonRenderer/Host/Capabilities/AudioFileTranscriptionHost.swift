import Foundation
import MetamonDSL

/// `fileImport` の accept audio: 選んだ録音ファイルを文字にする。モデルは音声入力・`transcribeField` と同じものをホストが持つ。
/// 音声も文字も端末の外に出さない。持たないホスト(プレビュー・テスト)では部品が「使えません」を出す。
@MainActor
public protocol AudioFileTranscriptionHost: AnyObject {
    /// 文字にできるか。`.preparing` は用意中・録音中・別の文字起こし中。
    var audioFileReadiness: DictationReadiness { get }
    /// モデルが無い・失敗した時: 用意を始める(ダウンロードの確認はホストが出す)。
    func prepareAudioFileTranscription()
    /// 端末内で文字にする(`progress` は 0...1)。Task のキャンセルで `CancellationError`。失敗は `DictationFailure`。
    func transcribeAudioFile(_ url: URL, progress: @escaping @MainActor (Double) -> Void) async throws -> String
}

public extension DSLRuntime {
    var audioFileTranscription: (any AudioFileTranscriptionHost)? { self as? any AudioFileTranscriptionHost }
}
