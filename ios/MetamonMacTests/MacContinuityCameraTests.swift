import XCTest
import AppKit
import MetamonDSL
@testable import MetamonRenderer
@testable import Metamon

/// 連係カメラ(#1852): iPhone で撮った写真・スキャンが `NSServicesMenuRequestor` で届き、photoField の値になる。
@MainActor
final class MacContinuityCameraTests: XCTestCase {
    private func pngData() throws -> Data {
        let rep = try XCTUnwrap(NSBitmapImageRep(bitmapDataPlanes: nil, pixelsWide: 40, pixelsHigh: 30, bitsPerSample: 8,
                                                 samplesPerPixel: 4, hasAlpha: true, isPlanar: false,
                                                 colorSpaceName: .deviceRGB, bytesPerRow: 0, bitsPerPixel: 0))
        NSGraphicsContext.saveGraphicsState()
        NSGraphicsContext.current = NSGraphicsContext(bitmapImageRep: rep)
        NSColor.systemTeal.setFill()
        NSRect(x: 0, y: 0, width: 40, height: 30).fill()
        NSGraphicsContext.restoreGraphicsState()
        return try XCTUnwrap(rep.representation(using: .png, properties: [:]))
    }

    private func board() -> NSPasteboard {
        let board = NSPasteboard(name: .init("metamon.test.continuity.\(UUID().uuidString)"))
        addTeardownBlock { board.releaseGlobally() }
        board.clearContents()
        return board
    }

    /// kakeibo の支出(photoField の欄 `receipt`)のドラフトと、それに入れる受け手。
    private func receiptForm() throws -> (FormDraft, ContinuityCameraRequestorView) {
        let url = try XCTUnwrap(Bundle(for: MacAppRuntime.self).url(forResource: "kakeibo", withExtension: "json"))
        let dsl = try AppDSL.load(contentsOf: url)
        let form = FormDraft(entity: try XCTUnwrap(dsl.entity("expense")))
        let requestor = ContinuityCameraRequestorView()
        requestor.onImage = { form.values["receipt"] = .string($0) }
        return (form, requestor)
    }

    func testRequestorAcceptsOnlyImageReturnTypes() {
        let view = ContinuityCameraRequestorView()
        XCTAssertTrue(view.validRequestor(forSendType: nil, returnType: .png) as? NSView === view)
        XCTAssertTrue(view.validRequestor(forSendType: nil, returnType: .tiff) as? NSView === view)
        XCTAssertTrue(view.validRequestor(forSendType: nil, returnType: .init("public.jpeg")) as? NSView === view)
        XCTAssertFalse(view.validRequestor(forSendType: nil, returnType: .string) as? NSView === view, "文字は受けない")
        XCTAssertFalse(view.validRequestor(forSendType: nil, returnType: .pdf) as? NSView === view, "PDF は受けない(スキャンも画像で受ける)")
        XCTAssertFalse(view.validRequestor(forSendType: .string, returnType: .png) as? NSView === view, "送る物は無い")
    }

    func testImageFromDeviceGoesIntoPhotoFieldValue() throws {
        let (form, requestor) = try receiptForm()
        let pb = board()
        pb.setData(try pngData(), forType: .png)
        XCTAssertTrue(requestor.readSelection(from: pb))
        let value = try XCTUnwrap(form.values["receipt"]?.stringValue)
        XCTAssertTrue(value.hasPrefix("data:image/jpeg;base64,"), "ファイル選択・貼り付けと同じ形")
        XCTAssertNotNil(PhotoCodec.image(from: .string(value)))
    }

    func testTiffFromDeviceAlsoGoesIn() throws {
        let (form, requestor) = try receiptForm()
        let pb = board()
        let tiff = try XCTUnwrap(NSImage(data: try pngData())?.tiffRepresentation)
        pb.setData(tiff, forType: .tiff)
        XCTAssertTrue(requestor.readSelection(from: pb))
        XCTAssertNotNil(PhotoCodec.image(from: form.values["receipt"] ?? .null))
    }

    func testNonImageLeavesFieldUntouched() throws {
        let (form, requestor) = try receiptForm()
        let pb = board()
        pb.setString("文字だけ", forType: .string)
        XCTAssertFalse(requestor.readSelection(from: pb))
        XCTAssertNil(form.values["receipt"]?.stringValue, "画像でなければ入れない")
    }

    func testMenuCarriesImportFromDevicesItem() {
        let menu = ContinuityCameraRequestorView.makeMenu()
        XCTAssertEqual(menu.items.map(\.identifier), [NSMenuItem.importFromDeviceIdentifier],
                       "AppKit が端末ごとの「写真を撮る / 書類をスキャン」に置き換える項目")
    }
}
