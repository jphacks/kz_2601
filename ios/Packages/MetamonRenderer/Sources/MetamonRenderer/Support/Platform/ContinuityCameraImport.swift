#if os(macOS)
import AppKit
import SwiftUI

/// 連係カメラ(#1852): 近くの iPhone で撮った写真・スキャンした書類を photoField に入れる。
/// メニューに `NSMenuItem.importFromDeviceIdentifier` を置くと、AppKit が端末ごとの「写真を撮る / 書類をスキャン」を足す。
/// 画像は `NSServicesMenuRequestor` の `readSelection(from:)` で届き、貼り付けと同じ `PhotoCodec` の data URL にする。
/// Mac のカメラ(#1043 `MacCameraSheet`)とは別の入口(Mac にカメラが無くても使える)。
@MainActor
enum ContinuityCameraImport {
    /// 受ける種類。画像だけ(PDF は受けない → 書類のスキャンも画像で届く)。
    static let returnTypes: [NSPasteboard.PasteboardType] = [
        .png, .tiff, NSPasteboard.PasteboardType("public.jpeg"), NSPasteboard.PasteboardType("public.heic"),
    ]

    static func accepts(sendType: NSPasteboard.PasteboardType?, returnType: NSPasteboard.PasteboardType?) -> Bool {
        guard sendType == nil, let returnType else { return false }
        return returnTypes.contains(returnType)
    }

    /// 届いた画像を photoField の値(data URL)にする。画像でなければ nil。
    static func dataURL(from board: NSPasteboard) -> String? {
        PhotoFileImport.dataURL(pasteboard: board)
    }
}

/// メニューを開く・画像を受ける AppKit の view。SwiftUI のボタンから `ContinuityCameraAnchor` 経由で開く。
final class ContinuityCameraRequestorView: NSView, NSServicesMenuRequestor {
    var onImage: (String) -> Void = { _ in }

    override var acceptsFirstResponder: Bool { true }

    override func validRequestor(forSendType sendType: NSPasteboard.PasteboardType?,
                                 returnType: NSPasteboard.PasteboardType?) -> Any? {
        if ContinuityCameraImport.accepts(sendType: sendType, returnType: returnType) { return self }
        return super.validRequestor(forSendType: sendType, returnType: returnType)
    }

    func readSelection(from pboard: NSPasteboard) -> Bool {
        guard let encoded = ContinuityCameraImport.dataURL(from: pboard) else { return false }
        onImage(encoded)
        return true
    }

    /// 端末の一覧のメニュー。AppKit が `importFromDeviceIdentifier` の項目を端末ごとの項目に置き換える。
    static func makeMenu() -> NSMenu {
        let menu = NSMenu()
        let item = NSMenuItem(title: L("iPhone から写真を撮る / 書類をスキャン"), action: nil, keyEquivalent: "")
        item.identifier = NSMenuItem.importFromDeviceIdentifier
        menu.addItem(item)
        return menu
    }

    func showMenu() {
        // 受け手を responder chain の先頭にしてから開く(届いた画像はこの view に来る)
        window?.makeFirstResponder(self)
        Self.makeMenu().popUp(positioning: nil, at: NSPoint(x: 0, y: bounds.height + 4), in: self)
    }
}

/// SwiftUI のボタンから AppKit の view に届くための入れ物。
@MainActor
final class ContinuityCameraAnchor {
    weak var view: ContinuityCameraRequestorView?
    func showMenu() { view?.showMenu() }
}

/// ボタンの背景に置く受け手。
struct ContinuityCameraRequestor: NSViewRepresentable {
    let anchor: ContinuityCameraAnchor
    let onImage: (String) -> Void

    func makeNSView(context: Context) -> ContinuityCameraRequestorView {
        let view = ContinuityCameraRequestorView()
        view.onImage = onImage
        anchor.view = view
        return view
    }

    func updateNSView(_ view: ContinuityCameraRequestorView, context: Context) {
        view.onImage = onImage
        anchor.view = view
    }
}
#endif
