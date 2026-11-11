import XCTest
import AVFoundation
import MetamonDSL
@testable import MetamonRenderer

/// #1339: videoField にファイルから入れる動画。受け付ける種類と、ユーザーのファイルを消さないこと。
final class VideoFileImportTests: XCTestCase {
    private var dir: URL!

    override func setUp() {
        dir = FileManager.default.temporaryDirectory.appendingPathComponent("videofile-\(UUID().uuidString)")
    }

    override func tearDown() {
        try? FileManager.default.removeItem(at: dir)
    }

    func testAcceptsMovieFilesOnly() {
        for name in ["a.mov", "a.MP4", "a.m4v", "a.qt"] {
            XCTAssertTrue(VideoFileImport.accepts(URL(fileURLWithPath: "/tmp/\(name)")), name)
        }
        for name in ["a.jpg", "a.pdf", "a.m4a", "a.txt", "a"] {
            XCTAssertFalse(VideoFileImport.accepts(URL(fileURLWithPath: "/tmp/\(name)")), name)
        }
        XCTAssertFalse(VideoFileImport.accepts(URL(string: "https://example.com/a.mp4")!), "ファイル以外は受けない")
    }

    func testOnlyTemporarySourceIsRemoved() {
        XCTAssertTrue(VideoFileImport.removesSource(.temporary))
        XCTAssertFalse(VideoFileImport.removesSource(.userFile))
    }

    func testIngestKeepsUserFileAndRemovesTemporary() async throws {
        let store = LocalVideoStore(directory: dir)

        let user = try await Self.makeMovie(seconds: 1)
        let v = try await VideoFileImport.ingest(user, origin: .userFile, maxDuration: 60, store: store)
        XCTAssertTrue(FileManager.default.fileExists(atPath: user.path), "ユーザーのファイルは残す")
        XCTAssertTrue(store.exists(try XCTUnwrap(v.id)))
        XCTAssertTrue(v.encoded.hasPrefix("video:"), "値は iOS と同じ video:<id>|<サムネイル>")
        try? FileManager.default.removeItem(at: user)

        let tmp = try await Self.makeMovie(seconds: 1)
        _ = try await VideoFileImport.ingest(tmp, origin: .temporary, maxDuration: 60, store: store)
        XCTAssertFalse(FileManager.default.fileExists(atPath: tmp.path), "一時ファイルは消す")
    }

    /// 単色フレームの H.264 動画を書く(10fps)。
    private static func makeMovie(seconds: Int) async throws -> URL {
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
