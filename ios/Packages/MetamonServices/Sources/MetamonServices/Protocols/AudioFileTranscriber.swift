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

/// 長い録音を区切る長さ(秒)。区切りごとに進み具合を出し、キャンセルを見る。
public enum AudioFileChunking {
    public static let blockSeconds = 120

    /// サンプル数 → 区切り(開始・終了の添字)。空なら []。
    public static func blocks(sampleCount: Int, sampleRate: Int) -> [Range<Int>] {
        let size = blockSeconds * sampleRate
        guard sampleCount > 0, size > 0 else { return [] }
        return stride(from: 0, to: sampleCount, by: size).map { $0..<min($0 + size, sampleCount) }
    }
}
