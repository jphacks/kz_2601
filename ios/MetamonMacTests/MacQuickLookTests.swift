import XCTest
import MetamonDSL
@testable import Metamon

/// 記録の写真・動画・録音を Quick Look で見る(#1850): 見せる物の並びと、渡すファイルの URL
@MainActor
final class MacQuickLookTests: XCTestCase {
    private var dir: URL!
    private var store: URL!

    override func setUp() {
        super.setUp()
        dir = FileManager.default.temporaryDirectory.appendingPathComponent("mac-ql-\(UUID().uuidString)")
        store = FileManager.default.temporaryDirectory.appendingPathComponent("mac-ql-store-\(UUID().uuidString)")
    }

    override func tearDown() {
        try? FileManager.default.removeItem(at: dir)
        try? FileManager.default.removeItem(at: store)
        super.tearDown()
    }

    private func fields(_ json: String) throws -> [Field] {
        try JSONDecoder().decode([Field].self, from: Data(json.utf8))
    }

    private let png = Data([0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A])
    private let jpeg = Data([0xFF, 0xD8, 0xFF, 0xE0])

    func testCollectsPhotosVideoAudioInFieldOrder() throws {
        let f = try fields("""
        [{"id":"title","name":"題","type":"string"},
         {"id":"photo","name":"表紙","type":"photo"},
         {"id":"secret","name":"正解","type":"photo","hidden":true},
         {"id":"remote","name":"ネット","type":"photo"},
         {"id":"clip","name":"動画","type":"video"},
         {"id":"voice","name":"録音","type":"audio"},
         {"id":"receipt","name":"レシート","type":"photo"}]
        """)
        let videos = LocalVideoStore(directory: store)
        let audio = LocalAudioStore(directory: store)
        let data: [String: JSONValue] = [
            "title": .string("本"),
            "photo": .string("data:image/png;base64," + png.base64EncodedString()),
            "secret": .string("data:image/jpeg;base64," + jpeg.base64EncodedString()),
            "remote": .string("https://example.com/a.jpg"),
            "clip": .string("video:clip1|data:image/jpeg;base64," + jpeg.base64EncodedString()),
            "voice": .string("audio:rec1|12"),
            "receipt": .string("data:image/jpeg;base64," + jpeg.base64EncodedString()),
        ]
        let sources = MacQuickLook.sources(data: data, fields: f, videos: videos, audio: audio, fileExists: { _ in true })
        XCTAssertEqual(sources, [
            .image(data: png, fileName: "表紙.png"),
            .file(try XCTUnwrap(videos.url(for: "clip1"))),
            .file(try XCTUnwrap(audio.url(for: "rec1"))),
            .image(data: jpeg, fileName: "レシート.jpg"),
        ], "隠す項目・https の写真・写真でない項目は出さない")
    }

    func testMissingVideoFallsBackToThumbnailAndMissingAudioIsSkipped() throws {
        let f = try fields("""
        [{"id":"clip","name":"動画","type":"video"},{"id":"voice","name":"録音","type":"audio"}]
        """)
        let data: [String: JSONValue] = [
            "clip": .string("video:clip1|data:image/jpeg;base64," + jpeg.base64EncodedString()),
            "voice": .string("audio:rec1|3"),
        ]
        let sources = MacQuickLook.sources(data: data, fields: f, videos: LocalVideoStore(directory: store),
                                           audio: LocalAudioStore(directory: store), fileExists: { _ in false })
        XCTAssertEqual(sources, [.image(data: jpeg, fileName: "動画.jpg")])
    }

    func testEmptyRecordHasNothingToShow() throws {
        let f = try fields(#"[{"id":"photo","name":"写真","type":"photo"}]"#)
        XCTAssertTrue(MacQuickLook.sources(data: [:], fields: f).isEmpty)
        XCTAssertTrue(MacQuickLook.sources(data: ["photo": .null], fields: f).isEmpty)
        XCTAssertNil(MacQuickLook.image("data:image/png;base64,", name: "x"), "中身の無い data URL は出さない")
    }

    func testURLsWritePhotosAndKeepFilesInOrder() throws {
        let file = store.appendingPathComponent("a.mp4")
        let urls = MacQuickLook.urls(for: [
            .image(data: png, fileName: "表紙.png"),
            .file(file),
            .image(data: jpeg, fileName: "表紙.png"),
        ], in: dir)
        XCTAssertEqual(urls.map(\.lastPathComponent), ["表紙.png", "a.mp4", "表紙 2.png"], "同じ名前は番号で分ける")
        XCTAssertEqual(try Data(contentsOf: urls[0]), png)
        XCTAssertEqual(try Data(contentsOf: urls[2]), jpeg)
        XCTAssertEqual(urls[1], file)
        XCTAssertTrue(urls[0].isFileURL)
    }

    func testURLsClearPreviousPreview() throws {
        _ = MacQuickLook.urls(for: [.image(data: png, fileName: "古い.png")], in: dir)
        _ = MacQuickLook.urls(for: [.image(data: jpeg, fileName: "新しい.jpg")], in: dir)
        let names = try FileManager.default.contentsOfDirectory(atPath: dir.path)
        XCTAssertEqual(names, ["新しい.jpg"], "前に見た写真は残さない")
    }

    func testSafeNameDropsPathCharacters() {
        XCTAssertEqual(MacQuickLook.safeName("a/b:c"), "abc")
        XCTAssertFalse(MacQuickLook.safeName("  ").isEmpty)
    }
}
