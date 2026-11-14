import XCTest
import AppKit
import MetamonServices
@testable import MetamonRenderer
@testable import Metamon

/// Mac の操作の残り(#1035): photoField のファイル・ドロップ・貼り付け、一覧全体の選択(⌘A)、ツールバーの「共有」。
@MainActor
final class MacInteractionsTests: XCTestCase {
    // MARK: - photoField(ファイル・ドロップ・⌘V)

    private func pngData(width: Int = 40, height: Int = 30) throws -> Data {
        let rep = try XCTUnwrap(NSBitmapImageRep(bitmapDataPlanes: nil, pixelsWide: width, pixelsHigh: height, bitsPerSample: 8,
                                                 samplesPerPixel: 4, hasAlpha: true, isPlanar: false,
                                                 colorSpaceName: .deviceRGB, bytesPerRow: 0, bitsPerPixel: 0))
        NSGraphicsContext.saveGraphicsState()
        NSGraphicsContext.current = NSGraphicsContext(bitmapImageRep: rep)
        NSColor.systemOrange.setFill()
        NSRect(x: 0, y: 0, width: width, height: height).fill()
        NSGraphicsContext.restoreGraphicsState()
        return try XCTUnwrap(rep.representation(using: .png, properties: [:]))
    }

    private func tempFile(_ data: Data, ext: String) throws -> URL {
        let url = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString).appendingPathExtension(ext)
        try data.write(to: url)
        addTeardownBlock { try? FileManager.default.removeItem(at: url) }
        return url
    }

    func testPhotoImportAcceptsOnlyImageFiles() {
        XCTAssertTrue(PhotoFileImport.accepts(URL(fileURLWithPath: "/tmp/a.png")))
        XCTAssertTrue(PhotoFileImport.accepts(URL(fileURLWithPath: "/tmp/b.JPG")))
        XCTAssertTrue(PhotoFileImport.accepts(URL(fileURLWithPath: "/tmp/c.heic")))
        XCTAssertFalse(PhotoFileImport.accepts(URL(fileURLWithPath: "/tmp/d.mov")))
        XCTAssertFalse(PhotoFileImport.accepts(URL(fileURLWithPath: "/tmp/e.txt")))
        XCTAssertFalse(PhotoFileImport.accepts(URL(string: "https://example.com/f.png")!), "ファイルでない URL は受けない")
    }

    func testPhotoImportReadsFileAsPhotoCodecDataURLAndKeepsSource() throws {
        let url = try tempFile(try pngData(), ext: "png")
        let encoded = try XCTUnwrap(PhotoFileImport.dataURL(fileAt: url))
        XCTAssertTrue(encoded.hasPrefix("data:image/jpeg;base64,"), "撮影・写真選択と同じ形")
        XCTAssertNotNil(PhotoCodec.image(from: .string(encoded)))
        XCTAssertTrue(FileManager.default.fileExists(atPath: url.path), "元のファイルは消さない")
    }

    func testPhotoImportRejectsNonImageFile() throws {
        XCTAssertNil(PhotoFileImport.dataURL(fileAt: try tempFile(Data("hello".utf8), ext: "txt")))
        XCTAssertNil(PhotoFileImport.dataURL(fileAt: try tempFile(Data("not a png".utf8), ext: "png")), "壊れた画像は入れない")
    }

    func testPhotoImportPastesImageFromPasteboard() throws {
        let board = NSPasteboard(name: .init("metamon.test.\(UUID().uuidString)"))
        defer { board.releaseGlobally() }
        board.clearContents()
        XCTAssertNil(PhotoFileImport.dataURL(pasteboard: board), "空なら入れない")
        board.setString("文字だけ", forType: .string)
        XCTAssertNil(PhotoFileImport.dataURL(pasteboard: board), "文字だけなら入れない")
        let image = try XCTUnwrap(NSImage(data: try pngData()))
        PlatformPasteboard.setImage(image, on: board)
        let encoded = try XCTUnwrap(PhotoFileImport.dataURL(pasteboard: board))
        XCTAssertTrue(encoded.hasPrefix("data:image/jpeg;base64,"))
    }

    // MARK: - 一覧全体(⌘A → ⌘C で CSV)

    func testSelectAllSelectsEveryRowForCSV() {
        var selection = ListRowMultiSelection()
        selection.select("b")
        selection.selectAll(order: ["a", "b", "c"])
        XCTAssertEqual(selection.ids, ["a", "b", "c"])
        XCTAssertTrue(selection.isMultiple, "⌘C は CSV になる")
        XCTAssertEqual(selection.ordered(["a", "b", "c"]), ["a", "b", "c"])
        XCTAssertEqual(selection.cursor, "c")
        // ⇧↑ は末尾から範囲を縮める(起点は先頭)
        selection.extend(.up, order: ["a", "b", "c"])
        XCTAssertEqual(selection.ids, ["a", "b"])
    }

    func testSelectAllOnSingleOrEmptyList() {
        var selection = ListRowMultiSelection()
        selection.selectAll(order: ["only"])
        XCTAssertEqual(selection.single, "only", "1 行なら 1 行のコピー(テキスト)のまま")
        selection.selectAll(order: [])
        XCTAssertTrue(selection.ids.isEmpty)
        XCTAssertNil(selection.cursor)
    }

    // MARK: - ツールバーの「共有」

    private actor FakeSharing: SharingClient {
        var issued = 0
        var revoked: [String] = []
        var fails = false
        func setFails(_ value: Bool) { fails = value }
        func share(appId: String) async throws -> ShareInfo {
            if fails { throw URLError(.notConnectedToInternet) }
            issued += 1
            return ShareInfo(token: "tok_\(issued)", appId: appId, name: "家計簿", dslVersion: 1,
                             url: "tailorit://import/tok_\(issued)", webUrl: "https://tailor-it.app/s/tok_\(issued)")
        }
        func revokeShare(token: String) async throws { revoked.append(token) }
        func importShare(token: String) async throws -> InstalledApp { throw URLError(.unsupportedURL) }
        func publish(appId: String, description: String, tags: [String], authorName: String?) async throws -> PublicAppListing {
            throw URLError(.unsupportedURL)
        }
        func unpublish(appId: String) async throws {}
        func searchPublicApps(query: String, sort: String) async throws -> [PublicAppListing] { [] }
        func install(publicAppId: String) async throws -> InstalledApp { throw URLError(.unsupportedURL) }
    }

    func testShareButtonEnabledOnlyForOwnServerAppWhenSignedIn() {
        XCTAssertTrue(MacShareAppModel.canShare(isCreated: true, isOnServer: true, signedIn: true, sharingAvailable: true))
        XCTAssertFalse(MacShareAppModel.canShare(isCreated: false, isOnServer: true, signedIn: true, sharingAvailable: true), "同梱の例")
        XCTAssertFalse(MacShareAppModel.canShare(isCreated: true, isOnServer: false, signedIn: true, sharingAvailable: true), "端末だけのアプリ")
        XCTAssertFalse(MacShareAppModel.canShare(isCreated: true, isOnServer: true, signedIn: false, sharingAvailable: true))
        XCTAssertFalse(MacShareAppModel.canShare(isCreated: true, isOnServer: true, signedIn: true, sharingAvailable: false))
    }

    func testShareHelpExplainsWhyDisabled() {
        XCTAssertEqual(MacShareAppModel.help(canShare: true, hasApp: true), L10n.tr("このアプリを共有する"))
        XCTAssertEqual(MacShareAppModel.help(canShare: false, hasApp: true), L10n.tr("サーバーに保存した自分のアプリだけ共有できます"))
        XCTAssertEqual(MacShareAppModel.help(canShare: false, hasApp: false), L10n.tr("アプリを選ぶと共有できます"))
    }

    func testShareLinkPrefersHttpsWebURL() {
        let info = ShareInfo(token: "t", appId: "a", name: "n", dslVersion: 1, url: "tailorit://import/t", webUrl: "https://tailor-it.app/s/t")
        XCTAssertEqual(MacShareAppModel.link(info)?.absoluteString, "https://tailor-it.app/s/t")
        var appOnly = info
        appOnly.webUrl = ""
        XCTAssertEqual(MacShareAppModel.link(appOnly)?.absoluteString, "tailorit://import/t")
    }

    func testShareIssuesCopiesAndRevokesLink() async throws {
        let fake = FakeSharing()
        let model = MacShareAppModel(appId: "app_1", appName: "家計簿", sharing: fake)
        await model.issue()
        XCTAssertEqual(model.link?.absoluteString, "https://tailor-it.app/s/tok_1")

        let board = NSPasteboard(name: .init("metamon.test.\(UUID().uuidString)"))
        defer { board.releaseGlobally() }
        XCTAssertTrue(model.copyLink(to: board))
        XCTAssertEqual(board.string(forType: .string), "https://tailor-it.app/s/tok_1")

        await model.revoke()
        let revoked = await fake.revoked
        XCTAssertEqual(revoked, ["tok_1"])
        XCTAssertEqual(model.link?.absoluteString, "https://tailor-it.app/s/tok_2", "取り消したら新しいリンクを用意する")
    }

    func testShareFailureAndOffline() async {
        let fake = FakeSharing()
        await fake.setFails(true)
        let model = MacShareAppModel(appId: "app_1", appName: "家計簿", sharing: fake)
        await model.issue()
        XCTAssertEqual(model.state, .failed(L10n.tr("リンクを作れませんでした。もう一度お試しください")))
        XCTAssertNil(model.link)
        XCTAssertFalse(model.copyLink(to: NSPasteboard(name: .init("metamon.test.\(UUID().uuidString)"))))

        let offline = MacShareAppModel(appId: "app_1", appName: "家計簿", sharing: nil)
        await offline.issue()
        XCTAssertEqual(offline.state, .failed(L10n.tr("シェアはサーバー接続時のみ使えます")))
    }
}
