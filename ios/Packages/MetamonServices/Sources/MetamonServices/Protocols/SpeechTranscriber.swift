import Foundation

/// 音声認識モデルの状態(UI 表示用)。
public enum TranscriberStatus: Equatable, Sendable {
    /// モデル未ダウンロード(prepare 前)
    case notDownloaded
    /// ダウンロード中(downloadProgress 0...1)
    case downloading
    /// ダウンロード済み・CoreML ロード/特化中
    case loading
    /// 録音・文字起こし可能
    case ready
    /// 文字起こし実行中
    case recognizing
    /// 準備失敗(prepare を再試行できる)
    case failed(String)
}

/// 音声入力。実機は `WhisperKitRecognizer`、シミュレータ/テストは `MockTranscriber`。
@MainActor
public protocol SpeechTranscriber: AnyObject {
    var isReady: Bool { get }
    var status: TranscriberStatus { get }
    /// モデル DL 進捗 0...1(downloading 以外は意味を持たない)
    var downloadProgress: Double { get }
    /// モデル DL / ロード。初回は時間がかかるのでアプリ起動直後に非同期で呼ぶ。
    func prepare() async throws
    func startRecording() async throws
    func stopAndTranscribe() async throws -> String
}

public extension SpeechTranscriber {
    var status: TranscriberStatus { isReady ? .ready : .loading }
    var downloadProgress: Double { 0 }
}
