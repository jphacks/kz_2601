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

    func testWindowsCoverWholeRecordingOnce() {
        let size = Double(AudioFileChunking.blockSeconds)
        XCTAssertEqual(AudioFileChunking.blockSeconds, 30, "背面に回った時に待つのは長くても 30 秒")
        XCTAssertEqual(AudioFileChunking.windows(seconds: 0), [])
        XCTAssertEqual(AudioFileChunking.windows(seconds: .nan), [])
        XCTAssertEqual(AudioFileChunking.windows(seconds: 10), [0..<10])
        let thirtyMin = 30.0 * 60
        let windows = AudioFileChunking.windows(seconds: thirtyMin + 5)
        XCTAssertEqual(windows.count, 61)
        XCTAssertEqual(windows.first, 0..<size)
        XCTAssertEqual(windows.last, (60 * size)..<(thirtyMin + 5))
        XCTAssertEqual(windows.map { $0.upperBound - $0.lowerBound }.reduce(0, +), thirtyMin + 5, accuracy: 1e-6)
        for (a, b) in zip(windows, windows.dropFirst()) { XCTAssertEqual(a.upperBound, b.lowerBound, "隙間・重なりなし") }
    }

    /// 区切りは 1 つずつ読んで文字にする(先に全部読まない)。無音の区切りは文字にしない。
    func testRunReadsOneWindowAtATime() async throws {
        let log = Log()
        var seen: [Double] = []
        let text = try await AudioFileChunking.run(
            seconds: 75,
            read: { window in
                await log.add("read \(Int(window.lowerBound))")
                return [Float](repeating: window.lowerBound == 30 ? 0 : 0.5, count: 4)
            },
            transcribe: { samples, _ in
                await log.add("transcribe")
                return samples.first == 0 ? "" : "part"
            },
            progress: { seen.append($0) }
        )
        XCTAssertEqual(text, "part\npart")
        let entries = await log.entries
        XCTAssertEqual(entries, ["read 0", "transcribe", "read 30", "transcribe", "read 60", "transcribe"])
        XCTAssertEqual(seen, [0, 1.0 / 3, 2.0 / 3, 1])
    }

    /// 文字にしている途中のキャンセル: `stop` で知らせ(WhisperKit の callback が false を返す)、次の区切りは読まない。
    func testRunStopsMidChunkOnCancel() async throws {
        let log = Log()
        let started = AsyncStream<Void>.makeStream()
        let job = Task { @MainActor in
            try await AudioFileChunking.run(
                seconds: 120,
                read: { window in
                    await log.add("read \(Int(window.lowerBound))")
                    return [0.5]
                },
                transcribe: { _, stop in
                    started.continuation.yield()
                    // WhisperKit の callback と同じく、stop を見るまで続ける(Task のキャンセルは見ない)
                    while !stop.isStopped { await Task.yield() }
                    return "途中まで"
                },
                progress: { _ in }
            )
        }
        for await _ in started.stream { break }
        job.cancel()
        do {
            _ = try await job.value
            XCTFail("want cancellation")
        } catch {
            XCTAssertTrue(error is CancellationError)
        }
        let entries = await log.entries
        XCTAssertEqual(entries, ["read 0"], "やめた後の区切りは読まない")
    }

    func testRunPropagatesReadFailure() async {
        struct Broken: Error {}
        do {
            _ = try await AudioFileChunking.run(seconds: 60, read: { _ in throw Broken() }, transcribe: { _, _ in "x" }, progress: { _ in })
            XCTFail("want failure")
        } catch {
            XCTAssertTrue(error is Broken)
        }
    }

    func testStatusAfterRecognizingKeepsOtherStates() {
        XCTAssertEqual(WhisperKitRecognizer.statusAfterRecognizing(.recognizing), .ready)
        XCTAssertEqual(WhisperKitRecognizer.statusAfterRecognizing(.failed("x")), .failed("x"), "失敗を ready で上書きしない")
        XCTAssertEqual(WhisperKitRecognizer.statusAfterRecognizing(.loading), .loading)
        XCTAssertEqual(WhisperKitRecognizer.statusAfterRecognizing(.notDownloaded), .notDownloaded)
    }

    actor Log {
        var entries: [String] = []
        func add(_ e: String) { entries.append(e) }
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
