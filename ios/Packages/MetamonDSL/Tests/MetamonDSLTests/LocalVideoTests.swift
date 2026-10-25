import XCTest
@testable import MetamonDSL

/// #206: video field の値と端末内の動画ファイル置き場。
final class LocalVideoTests: XCTestCase {
    func testParseAndEncode() throws {
        let v = try XCTUnwrap(VideoValue(string: "video:ABC-1_2|data:image/jpeg;base64,xx"))
        XCTAssertEqual(v.id, "ABC-1_2")
        XCTAssertEqual(v.thumbnail, "data:image/jpeg;base64,xx")
        XCTAssertEqual(v.encoded, "video:ABC-1_2|data:image/jpeg;base64,xx")

        let bare = try XCTUnwrap(VideoValue(.string("video:abc")))
        XCTAssertNil(bare.thumbnail)
        XCTAssertEqual(bare.encoded, "video:abc")

        XCTAssertEqual(VideoValue(string: "https://example.com/v.mp4")?.remoteURL?.host, "example.com")
        for bad in ["", "abc", "video:", "video:../x", "video:a/b", "data:image/jpeg;base64,xx"] {
            XCTAssertNil(VideoValue(string: bad), bad)
        }
        XCTAssertNil(VideoValue(.number(1)))
    }

    func testLocalIDs() {
        let data: [String: JSONValue] = ["v": .string("video:a|data:image/jpeg;base64,x"), "w": .string("video:b"),
                                         "photo": .string("data:image/jpeg;base64,x"), "n": .number(1)]
        XCTAssertEqual(VideoValue.localIDs(in: data), ["a", "b"])
    }

    func testStoreImportAndRemove() throws {
        let dir = FileManager.default.temporaryDirectory.appendingPathComponent("videos-\(UUID().uuidString)")
        let store = LocalVideoStore(directory: dir)
        defer { store.removeAll() }
        let src = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString + ".mp4")
        try Data([0, 1, 2]).write(to: src)

        let id = try store.importFile(at: src)
        XCTAssertTrue(store.exists(id))
        XCTAssertNil(store.url(for: "../escape"))
        store.remove([id])
        XCTAssertFalse(store.exists(id))
    }

    func testDisplayAndDecode() throws {
        let f = try JSONDecoder().decode(Field.self, from: Data(#"{"id":"v","name":"動画","type":"video"}"#.utf8))
        XCTAssertEqual(f.type, .video)
        let c = try JSONDecoder().decode(Component.self, from: Data(#"{"type":"videoField","field":"v","source":"camera","maxDuration":30}"#.utf8))
        XCTAssertEqual(c.type, .videoField)
        XCTAssertEqual(c.maxDuration, 30)
        XCTAssertEqual(FieldCoding.display(.string("video:a"), field: f), "動画")
        XCTAssertEqual(FieldCoding.display(.null, field: f), "")
    }
}
