import XCTest
import AVFoundation
import UIKit
import MetamonDSL
@testable import MetamonRenderer

/// #206: 動画の取り込み(mp4 に書き出し → 端末内に保存 → 1 フレーム目のサムネイル)。
/// シミュレータではカメラで撮れないので、AVAssetWriter で作った 3 秒の動画を「ライブラリから選んだ動画」の代わりに使う。
final class VideoCodecTests: XCTestCase {
    private var dir: URL!

    override func setUp() {
        dir = FileManager.default.temporaryDirectory.appendingPathComponent("videos-\(UUID().uuidString)")
    }

    override func tearDown() {
        try? FileManager.default.removeItem(at: dir)
    }

    func testIngestStoresFileTrimsAndMakesSmallThumbnail() async throws {
        let src = try await Self.makeMovie(seconds: 3)
        let store = LocalVideoStore(directory: dir)

        let v = try await VideoCodec.ingest(src, maxDuration: 1, store: store)
        let id = try XCTUnwrap(v.id)
        XCTAssertTrue(store.exists(id))
        let thumb = try XCTUnwrap(v.thumbnail)
        XCTAssertTrue(thumb.hasPrefix("data:image/jpeg;base64,"))
        XCTAssertLessThan(v.encoded.utf8.count, 60_000, "レコードに入れる値は数十 KB 以内")
        XCTAssertNotNil(PhotoCodec.image(from: .string(v.encoded)), "一覧の item.image でサムネイルが出る")

        let duration = try await AVURLAsset(url: try XCTUnwrap(store.url(for: id))).load(.duration)
        XCTAssertLessThanOrEqual(CMTimeGetSeconds(duration), 1.2, "maxDuration 秒に切る")
    }

    func testRegistryHasVideoComponents() {
        let types = ComponentRegistry.standard.registeredTypes
        XCTAssertTrue(types.contains("videoField"))
        XCTAssertTrue(types.contains("videoPlayer"))
    }

    /// 単色フレームの H.264 動画を書く(10fps)。
    static func makeMovie(seconds: Int) async throws -> URL {
        let url = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString).appendingPathExtension("mov")
        let writer = try AVAssetWriter(outputURL: url, fileType: .mov)
        let size = CGSize(width: 320, height: 240)
        let input = AVAssetWriterInput(mediaType: .video, outputSettings: [
            AVVideoCodecKey: AVVideoCodecType.h264, AVVideoWidthKey: size.width, AVVideoHeightKey: size.height,
        ])
        let adaptor = AVAssetWriterInputPixelBufferAdaptor(assetWriterInput: input, sourcePixelBufferAttributes: [
            kCVPixelBufferPixelFormatTypeKey as String: kCVPixelFormatType_32ARGB,
            kCVPixelBufferWidthKey as String: size.width, kCVPixelBufferHeightKey as String: size.height,
        ])
        writer.add(input)
        writer.startWriting()
        writer.startSession(atSourceTime: .zero)
        for i in 0..<(seconds * 10) {
            while !input.isReadyForMoreMediaData { try await Task.sleep(nanoseconds: 5_000_000) }
            var buffer: CVPixelBuffer?
            CVPixelBufferPoolCreatePixelBuffer(nil, adaptor.pixelBufferPool!, &buffer)
            let pb = try XCTUnwrap(buffer)
            CVPixelBufferLockBaseAddress(pb, [])
            memset(CVPixelBufferGetBaseAddress(pb), Int32(i * 8 % 255), CVPixelBufferGetDataSize(pb))
            CVPixelBufferUnlockBaseAddress(pb, [])
            adaptor.append(pb, withPresentationTime: CMTime(value: CMTimeValue(i), timescale: 10))
        }
        input.markAsFinished()
        await writer.finishWriting()
        XCTAssertEqual(writer.status, .completed)
        return url
    }
}
