import XCTest
@testable import Metamon

/// #2224: 共有シートで受け取った物の置き場。App Group のフォルダを一時フォルダに差し替えて置く・読む・消すを見る。
final class ShareInboxTests: XCTestCase {
    private var directory: URL!
    private var inbox: ShareInbox!

    override func setUpWithError() throws {
        try super.setUpWithError()
        directory = URL.temporaryDirectory.appendingPathComponent("ShareInboxTests-\(UUID().uuidString)", isDirectory: true)
        inbox = ShareInbox(directory: directory)
    }

    override func tearDownWithError() throws {
        try? FileManager.default.removeItem(at: directory)
        try super.tearDownWithError()
    }

    func testPutAndReadText() throws {
        try inbox.put(kind: .text, text: "めも")
        let items = inbox.items()
        XCTAssertEqual(items.count, 1)
        XCTAssertEqual(items.first?.kind, .text)
        XCTAssertEqual(items.first?.text, "めも")
        XCTAssertNil(items.first?.fileName)
    }

    func testPutFileKeepsData() throws {
        let data = Data("%PDF-1.4".utf8)
        let item = try inbox.put(kind: .pdf, data: data, fileExtension: "pdf")
        let file = try XCTUnwrap(inbox.fileURL(of: item))
        XCTAssertEqual(file.pathExtension, "pdf")
        XCTAssertEqual(try Data(contentsOf: file), data)
    }

    func testItemsAreNewestFirst() throws {
        let old = Date(timeIntervalSince1970: 1_000)
        try inbox.put(kind: .text, text: "古い", now: old)
        try inbox.put(kind: .url, text: "https://example.com", now: old.addingTimeInterval(60))
        XCTAssertEqual(inbox.items().map(\.kind), [.url, .text])
    }

    func testRemoveDeletesMetaAndFile() throws {
        let item = try inbox.put(kind: .image, data: Data([0x01]), fileExtension: "png")
        let file = try XCTUnwrap(inbox.fileURL(of: item))
        inbox.remove(item)
        XCTAssertTrue(inbox.items().isEmpty)
        XCTAssertFalse(FileManager.default.fileExists(atPath: file.path))
    }

    func testRemoveAll() throws {
        try inbox.put(kind: .text, text: "1")
        try inbox.put(kind: .text, text: "2")
        inbox.removeAll()
        XCTAssertTrue(inbox.items().isEmpty)
    }

    /// まだ何も置いていない(フォルダが無い)時も空で読める
    func testMissingDirectoryReadsEmpty() {
        XCTAssertTrue(inbox.items().isEmpty)
    }

    @MainActor
    func testUIOpensOnlyForInboxURL() throws {
        try inbox.put(kind: .text, text: "めも")
        let ui = ShareInboxUI(inbox: inbox)
        XCTAssertFalse(ui.receive(URL(string: "tailorit://import/tok_1")!))
        XCTAssertFalse(ui.isShowing)
        XCTAssertTrue(ui.receive(ShareInbox.openURL))
        XCTAssertTrue(ui.isShowing)
        XCTAssertEqual(ui.items.count, 1)
        ui.removeAll()
        XCTAssertTrue(ui.items.isEmpty)
    }

    func testOpenURLIsRecognised() {
        XCTAssertTrue(ShareInbox.isOpenURL(ShareInbox.openURL))
        XCTAssertTrue(ShareInbox.isOpenURL(URL(string: "TailorIt://Share-Inbox")!))
        XCTAssertFalse(ShareInbox.isOpenURL(URL(string: "tailorit://import/tok_1")!))
        XCTAssertFalse(ShareInbox.isOpenURL(URL(string: "https://tailor-it.app/share-inbox")!))
    }
}
