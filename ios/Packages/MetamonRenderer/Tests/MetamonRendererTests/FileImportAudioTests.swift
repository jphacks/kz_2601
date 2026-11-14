import UniformTypeIdentifiers
import XCTest
@testable import MetamonDSL
@testable import MetamonRenderer

/// `fileImport` の accept audio: 選べる種類・録音ファイルの見分け・上限。
final class FileImportAudioTests: XCTestCase {
    func testContentTypesIncludeAudio() {
        XCTAssertEqual(FileTextReading.contentTypes(["pdf", "audio"]), [.pdf, .audio])
    }

    func testKindOfRecordings() {
        for name in ["授業.m4a", "会議.MP3", "a.wav", "a.aiff"] {
            XCTAssertEqual(FileTextReading.kind(of: URL(fileURLWithPath: "/tmp/\(name)"), accept: ["audio"]), .audio, name)
        }
        XCTAssertNil(FileTextReading.kind(of: URL(fileURLWithPath: "/tmp/a.m4a"), accept: ["pdf", "text"]))
        XCTAssertNil(FileTextReading.kind(of: URL(fileURLWithPath: "/tmp/a.mp4"), accept: ["audio"]), "video is not audio")
        XCTAssertEqual(FileTextReading.kind(of: URL(fileURLWithPath: "/tmp/a.pdf"), accept: ["pdf", "audio"]), .pdf)
    }

    func testLimits() {
        XCTAssertNil(AudioFileImport.check(bytes: 10_000_000, seconds: 25 * 60))
        XCTAssertNil(AudioFileImport.check(bytes: nil, seconds: 30 * 60))
        XCTAssertEqual(AudioFileImport.check(bytes: 10_000_000, seconds: 30 * 60 + 1), .tooLong)
        XCTAssertEqual(AudioFileImport.check(bytes: 200_000_001, seconds: 60), .tooLarge)
        XCTAssertEqual(AudioFileImport.check(bytes: 1000, seconds: nil), .unreadable)
        XCTAssertEqual(AudioFileImport.check(bytes: 1000, seconds: .nan), .unreadable)
        XCTAssertEqual(AudioFileImport.check(bytes: 1000, seconds: 0), .empty)
    }

    func testUnreadableFileHasNoDuration() async {
        let url = FileManager.default.temporaryDirectory.appending(path: "fa-\(UUID().uuidString).m4a")
        try? Data("not audio".utf8).write(to: url)
        defer { try? FileManager.default.removeItem(at: url) }
        let m = await AudioFileImport.measure(url)
        XCTAssertEqual(m.bytes, 9)
        XCTAssertNotNil(AudioFileImport.check(bytes: m.bytes, seconds: m.seconds))
    }

    /// ホストが無い(プレビュー・テスト)ランタイムでは audio の文字起こしを使えない。
    @MainActor
    func testNoHostWithoutTranscriber() {
        XCTAssertNil(UnboundDSLRuntime.shared.audioFileTranscription)
    }
}
