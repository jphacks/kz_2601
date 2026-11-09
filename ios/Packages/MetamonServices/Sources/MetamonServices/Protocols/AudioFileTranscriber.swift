import Foundation

/// 録音ファイルの文字起こし(`fileImport` の accept audio)。音声入力と同じ端末内のモデル(WhisperKit)で、音声も文字も端末の外に出さない。
@MainActor
public protocol AudioFileTranscriber: AnyObject {
    /// 今すぐ文字にできるか(本命のモデルが用意済みで、録音・別の文字起こしをしていない)。
    var canTranscribeFiles: Bool { get }
    /// ファイルを読み(16kHz モノラル)、端末内で文字にする。`progress` は 0...1。
    /// Task のキャンセルで止まる(`CancellationError`)。声が無ければ ""。
    func transcribeFile(at url: URL, progress: @escaping @MainActor (Double) -> Void) async throws -> String
}

/// 長い録音を区切って、区切りごとに読み・文字にする(ファイル全体をメモリに載せない)。
public enum AudioFileChunking {
    /// 区切りの長さ(秒)。背面に回った時に今の区切りを待つ長さの上限でもある(Whisper の窓と同じ 30 秒)。
    public static let blockSeconds = 30

    /// 録音の長さ(秒) → 区切り(開始・終了の秒)。空・不正なら []。
    public static func windows(seconds: Double) -> [Range<Double>] {
        guard seconds > 0, seconds.isFinite else { return [] }
        let size = Double(blockSeconds)
        let count = Int((seconds / size).rounded(.up))
        return (0..<count).map { i in (Double(i) * size)..<min(Double(i + 1) * size, seconds) }
    }

    /// 区切りを順に `read`(端末内で 16kHz モノラルに読む)→ `transcribe` して、文字をつなぐ。
    /// 読んだ区切りは次へ進む前に手放す。Task のキャンセルは区切りごとに見て、文字にしている途中は `stop` で知らせる
    /// (`transcribe` はこれを見て早めにやめる)。キャンセルされたら `CancellationError`。
    @MainActor
    public static func run(
        seconds: Double,
        read: @escaping @Sendable (Range<Double>) async throws -> [Float],
        transcribe: ([Float], AudioFileStop) async throws -> String,
        progress: (Double) -> Void
    ) async throws -> String {
        let windows = windows(seconds: seconds)
        let stop = AudioFileStop()
        return try await withTaskCancellationHandler {
            var parts: [String] = []
            progress(0)
            for (i, window) in windows.enumerated() {
                try Task.checkCancellation()
                let text = try await transcribe(try await read(window), stop)
                if stop.isStopped { throw CancellationError() }
                try Task.checkCancellation()
                if !text.isEmpty { parts.append(text) }
                progress(Double(i + 1) / Double(windows.count))
            }
            return parts.joined(separator: "\n")
        } onCancel: {
            stop.stop()
        }
    }
}

/// 文字起こしの途中でやめる合図(キャンセルの知らせはどのスレッドからも来る)。
public final class AudioFileStop: @unchecked Sendable {
    private let lock = NSLock()
    private var stopped = false

    public init() {}

    public var isStopped: Bool { lock.withLock { stopped } }

    public func stop() { lock.withLock { stopped = true } }
}
