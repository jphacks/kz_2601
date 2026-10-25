import AVFoundation
import UIKit
import MetamonDSL

/// 撮影・選択した動画を端末内に保存し、video field の値(`VideoValue`)を作る(#206)。
/// - 中程度の画質(`AVAssetExportPresetMediumQuality`)の mp4 に書き出して `LocalVideoStore` へ
/// - 長い動画は先頭 `maxDuration` 秒に切る(ライブラリから選んだ時。カメラは撮影側で上限)
/// - 1 フレーム目を 320px・q0.5 に縮小した data URL をサムネイルにする(数十 KB)
public enum VideoCodec {
    public static let defaultMaxDuration = 60
    public static let thumbnailMaxSide: CGFloat = 320

    public enum Failure: Error { case exportFailed(String) }

    public static func ingest(_ source: URL, maxDuration: Int? = nil, store: LocalVideoStore = .shared) async throws -> VideoValue {
        let limit = Double(max(1, maxDuration ?? defaultMaxDuration))
        let asset = AVURLAsset(url: source)
        let tmp = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString).appendingPathExtension("mp4")
        try await export(asset, to: tmp, limit: limit)
        let id = try store.importFile(at: tmp)
        guard let url = store.url(for: id) else { throw Failure.exportFailed("no url") }
        let thumb = await thumbnail(for: url)
        return VideoValue(id: id, thumbnail: thumb)
    }

    /// 1 フレーム目の縮小 JPEG(data URL)。取れなければ nil(サムネイル無しで保存する)。
    public static func thumbnail(for url: URL) async -> String? {
        let gen = AVAssetImageGenerator(asset: AVURLAsset(url: url))
        gen.appliesPreferredTrackTransform = true
        gen.maximumSize = CGSize(width: thumbnailMaxSide, height: thumbnailMaxSide)
        guard let cg = try? await gen.image(at: .zero).image else { return nil }
        return PhotoCodec.dataURL(from: UIImage(cgImage: cg), maxSide: thumbnailMaxSide, quality: 0.5)
    }

    private static func export(_ asset: AVAsset, to dest: URL, limit: Double) async throws {
        guard let session = AVAssetExportSession(asset: asset, presetName: AVAssetExportPresetMediumQuality) else {
            throw Failure.exportFailed("no session")
        }
        session.outputURL = dest
        session.outputFileType = .mp4
        session.shouldOptimizeForNetworkUse = true
        let duration = (try? await asset.load(.duration)).map(CMTimeGetSeconds) ?? 0
        if duration > limit {
            session.timeRange = CMTimeRange(start: .zero, duration: CMTime(seconds: limit, preferredTimescale: 600))
        }
        await withCheckedContinuation { (cont: CheckedContinuation<Void, Never>) in
            session.exportAsynchronously { cont.resume() }
        }
        guard session.status == .completed else {
            throw Failure.exportFailed(session.error?.localizedDescription ?? "status \(session.status.rawValue)")
        }
    }
}
