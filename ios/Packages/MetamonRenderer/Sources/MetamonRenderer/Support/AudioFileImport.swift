import AVFoundation
import Foundation

/// `fileImport` の accept audio: 録音ファイルの上限と下見(長さ・大きさ)。文字にするのはホスト(`AudioFileTranscriptionHost`)。
/// 上限は backend `dsl.MaxImportAudioMinutes` / `MaxImportAudioMB` と同じ。
enum AudioFileImport {
    static let maxSeconds: Double = 30 * 60
    static let maxBytes = 200 * 1_000_000

    enum Problem: Equatable {
        case tooLarge, tooLong, unreadable, empty
    }

    /// 大きさ・長さが上限に収まるか。収まれば nil。
    static func check(bytes: Int?, seconds: Double?) -> Problem? {
        if let bytes, bytes > maxBytes { return .tooLarge }
        guard let seconds, seconds.isFinite else { return .unreadable }
        if seconds <= 0 { return .empty }
        if seconds > maxSeconds { return .tooLong }
        return nil
    }

    /// ファイルの大きさ(バイト)と長さ(秒)。読めなければ nil。
    static func measure(_ url: URL) async -> (bytes: Int?, seconds: Double?) {
        let bytes = try? url.resourceValues(forKeys: [.fileSizeKey]).fileSize
        let asset = AVURLAsset(url: url)
        let duration = try? await asset.load(.duration)
        return (bytes, duration.map(\.seconds))
    }

    /// 「12 分 30 秒」のような長さ。
    static func durationText(_ seconds: Double) -> String {
        let s = max(0, Int(seconds.rounded()))
        return s >= 60 ? L("\(s / 60) 分 \(s % 60) 秒") : L("\(s) 秒")
    }
}
