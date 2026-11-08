import XCTest
@testable import MetamonServices

/// `fileImport` accept audio: 長い録音の区切り方と、本命(WhisperKit)だけで文字にすること。
@MainActor
final class AudioFileTranscriberTests: XCTestCase {
    final class FakeFilePrimary: SpeechTranscriber, AudioFileTranscriber {
        var isReady = true
        var canTranscribeFiles = true
        var files: [URL] = []
        func prepare() async throws {}
        func startRecording() async throws {}
        func stopAndTranscribe() async throws -> String { "" }
        func transcribeFile(at url: URL, progress: @escaping @MainActor (Double) -> Void) async throws -> String {
            files.append(url)
            progress(1)
            return "講義の文字起こし"
        }
    }

    func testBlocksCoverEverySampleOnce() {
        let rate = 16_000, block = AudioFileChunking.blockSeconds * rate
        XCTAssertEqual(AudioFileChunking.blocks(sampleCount: 0, sampleRate: rate), [])
        XCTAssertEqual(AudioFileChunking.blocks(sampleCount: 10, sampleRate: rate), [0..<10])
        let thirtyMin = 30 * 60 * rate
        let blocks = AudioFileChunking.blocks(sampleCount: thirtyMin + 5, sampleRate: rate)
        XCTAssertEqual(blocks.count, 16)
        XCTAssertEqual(blocks.first, 0..<block)
        XCTAssertEqual(blocks.last, (15 * block)..<(thirtyMin + 5))
        XCTAssertEqual(blocks.map(\.count).reduce(0, +), thirtyMin + 5)
    }

    func testFallbackUsesPrimaryOnly() async throws {
        let primary = FakeFilePrimary(), apple = FallbackTranscriberTests.FakeProvisional()
        let t = FallbackTranscriber(primary: primary, provisional: apple)
        XCTAssertTrue(t.canTranscribeFiles)
        var seen: [Double] = []
        let text = try await t.transcribeFile(at: URL(fileURLWithPath: "/tmp/a.m4a")) { seen.append($0) }
        XCTAssertEqual(text, "講義の文字起こし")
        XCTAssertEqual(seen, [1])
        XCTAssertEqual(apple.started, 0, "仮の聞き取りは録音ファイルに使わない")
    }

    func testFallbackRefusesWhilePrimaryIsNotReady() async {
        let primary = FakeFilePrimary(), apple = FallbackTranscriberTests.FakeProvisional()
        primary.canTranscribeFiles = false
        let t = FallbackTranscriber(primary: primary, provisional: apple)
        XCTAssertFalse(t.canTranscribeFiles, "仮の聞き取りが使えても、録音ファイルは本命の準備を待つ")
        do {
            _ = try await t.transcribeFile(at: URL(fileURLWithPath: "/tmp/a.m4a")) { _ in }
            XCTFail("want notReady")
        } catch {
            XCTAssertTrue(error is TranscriberFailure)
        }
    }

    func testFallbackRefusesWhileRecording() async throws {
        let primary = FakeFilePrimary(), apple = FallbackTranscriberTests.FakeProvisional()
        let t = FallbackTranscriber(primary: primary, provisional: apple)
        try await t.startRecording()
        XCTAssertFalse(t.canTranscribeFiles)
    }

    func testMockReportsProgressAndCancels() async throws {
        let mock = MockTranscriber(delay: .milliseconds(1))
        var seen: [Double] = []
        let text = try await mock.transcribeFile(at: URL(fileURLWithPath: "/tmp/会議.m4a")) { seen.append($0) }
        XCTAssertEqual(seen, [0.25, 0.5, 0.75, 1])
        XCTAssertTrue(text.contains("会議.m4a"))

        let slow = MockTranscriber(delay: .seconds(5))
        let job = Task { try await slow.transcribeFile(at: URL(fileURLWithPath: "/tmp/a.m4a")) { _ in } }
        job.cancel()
        do {
            _ = try await job.value
            XCTFail("want cancellation")
        } catch {
            XCTAssertTrue(error is CancellationError)
        }
    }
}
