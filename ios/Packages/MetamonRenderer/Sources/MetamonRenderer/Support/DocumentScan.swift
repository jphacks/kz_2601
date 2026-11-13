import Foundation
import PDFKit
#if canImport(UIKit)
import UIKit
#else
import AppKit
#endif
#if canImport(VisionKit) && canImport(UIKit)
import VisionKit
#endif
import MetamonDSL

/// 書類をスキャンして、傾きを直したページの画像・PDF を返す口(#2227・親 #1826)。
/// 部品・式・AI の決まり・`supportedFeatures` の申告はまだ無い(#1775 の決定待ち)。
/// `ComposeMessage` / `DeviceCalendar` と同じ形: protocol + 実体(`LiveDocumentScanner`)+ 偽物(`FakeDocumentScanner`)。
///
/// 画面(`VNDocumentCameraViewController`)は単体テストできないので、
/// 「ページの画像の並び → PDF」は `DocumentScanPDF` の純関数に分けてある(Mac でもテストできる)。
///
/// 撮った中身は端末の中だけで扱う。ファイルにも保存せず、ログにも出さない。

// MARK: - 画像の並び → PDF(純関数・単体テストの対象)

public enum DocumentScanPDF {
    /// 1 ページの長辺の上限(px)。写真の部品の保存(`PhotoCodec.dataURL`)と同じ。
    public static let defaultMaxSide: CGFloat = 800
    /// JPEG の圧縮。写真の部品の保存(`PhotoCodec.dataURL`)と同じ。
    public static let defaultQuality: CGFloat = 0.5

    /// ページの画像の並びを 1 つの PDF にする。並びの順がそのままページの順になる。
    ///
    /// 空の並びは nil(0 ページの PDF は作らない)。
    /// スキャンをやめた時も画像は 0 枚で返ってくるので、「やめた」と「中身の無い PDF」を
    /// 呼び出し側が区別できるよう、どちらも「PDF は無い」に寄せる。
    /// 縮小・圧縮に全部こけた時も同じく nil。
    public static func data(from images: [PlatformImage],
                           maxSide: CGFloat = defaultMaxSide,
                           quality: CGFloat = defaultQuality) -> Data? {
        let document = PDFDocument()
        for image in images {
            guard let shrunk = compressed(image, maxSide: maxSide, quality: quality),
                  let page = PDFPage(image: shrunk) else { continue }
            document.insert(page, at: document.pageCount)
        }
        guard document.pageCount > 0 else { return nil }
        return document.dataRepresentation()
    }

    /// 1 枚目だけを写真の部品と同じ値(`data:image/jpeg;base64,…`)で返す道。空の並びは nil。
    public static func firstImageValue(from images: [PlatformImage],
                                       maxSide: CGFloat = defaultMaxSide,
                                       quality: CGFloat = defaultQuality) -> String? {
        guard let first = images.first else { return nil }
        return PhotoCodec.dataURL(from: first, maxSide: maxSide, quality: quality)
    }

    /// 写真の部品の保存とまったく同じ縮小・圧縮を通した画像。
    /// 上限・圧縮がずれないよう `PhotoCodec` を通す(自前で同じ計算を書き写さない)。
    static func compressed(_ image: PlatformImage, maxSide: CGFloat, quality: CGFloat) -> PlatformImage? {
        guard let dataURL = PhotoCodec.dataURL(from: image, maxSide: maxSide, quality: quality) else { return nil }
        return PhotoCodec.image(from: .string(dataURL))
    }
}

// MARK: - スキャンする口

@MainActor
public protocol DocumentScanning {
    /// この端末で書類のスキャンが使えるか(Mac・古い端末・カメラの無い端末では false)。
    var isSupported: Bool { get }
    /// スキャンの画面を出して、閉じるまで待つ。ページの画像を撮った順に返す。
    /// やめた時・出せなかった時は空の並び。
    func scan() async -> [PlatformImage]
}

@MainActor
public enum DocumentScan {
    /// スキャンする口の実体。テスト・プレビューでは差し替える。
    public static var scanner: any DocumentScanning = LiveDocumentScanner()

    public static var isSupported: Bool { scanner.isSupported }

    /// ページの画像を撮った順に返す。
    public static func pages() async -> [PlatformImage] { await pages(using: scanner) }

    /// スキャンした全ページの PDF。やめた時・使えない時は nil。
    public static func pdf() async -> Data? { await pdf(using: scanner) }

    /// スキャンの 1 枚目だけを写真の部品と同じ値で返す。やめた時・使えない時は nil。
    public static func firstImageValue() async -> String? { await firstImageValue(using: scanner) }

    /// 以下はスキャンする口を差し替えられる形(単体テストはこちらを呼ぶ)。
    public static func pages(using scanner: any DocumentScanning) async -> [PlatformImage] {
        guard scanner.isSupported else { return [] }
        return await scanner.scan()
    }

    public static func pdf(using scanner: any DocumentScanning) async -> Data? {
        DocumentScanPDF.data(from: await pages(using: scanner))
    }

    public static func firstImageValue(using scanner: any DocumentScanning) async -> String? {
        DocumentScanPDF.firstImageValue(from: await pages(using: scanner))
    }
}

// MARK: - 実体

/// iOS では `VNDocumentCameraViewController`(OS が端・傾きを直したページを返す)。Mac では使えない。
@MainActor
public final class LiveDocumentScanner: DocumentScanning {
    public init() {}

    public var isSupported: Bool {
        #if canImport(VisionKit) && canImport(UIKit)
        VNDocumentCameraViewController.isSupported
        #else
        false
        #endif
    }

    public func scan() async -> [PlatformImage] {
        #if canImport(VisionKit) && canImport(UIKit)
        guard isSupported, let host = Self.topViewController() else { return [] }
        let bridge = DocumentScanDelegateBridge()
        let controller = VNDocumentCameraViewController()
        controller.delegate = bridge
        return await withCheckedContinuation { continuation in
            bridge.finish = { [weak controller] images in
                controller?.dismiss(animated: true)
                continuation.resume(returning: images)
            }
            host.present(controller, animated: true)
        }
        #else
        return []
        #endif
    }

    #if canImport(UIKit)
    /// 今いちばん上の画面(そこにスキャンの画面を重ねる)。
    static func topViewController() -> UIViewController? {
        let window = UIApplication.shared.connectedScenes
            .compactMap { ($0 as? UIWindowScene)?.keyWindow }
            .first
        var top = window?.rootViewController
        while let next = top?.presentedViewController { top = next }
        return top
    }
    #endif
}

#if canImport(VisionKit) && canImport(UIKit)
/// `VNDocumentCameraViewController` の 3 つのデリゲートを「ページの画像の並び」1 つにまとめる。
/// 閉じるまで自分で自分を持つ(`delegate` は weak なので)。
@MainActor
private final class DocumentScanDelegateBridge: NSObject, @preconcurrency VNDocumentCameraViewControllerDelegate {
    var finish: (([PlatformImage]) -> Void)?
    private var retain: DocumentScanDelegateBridge?

    override init() {
        super.init()
        retain = self
    }

    private func done(_ images: [PlatformImage]) {
        let finish = self.finish
        self.finish = nil
        retain = nil
        finish?(images)
    }

    func documentCameraViewController(_ controller: VNDocumentCameraViewController,
                                      didFinishWith scan: VNDocumentCameraScan) {
        done((0..<scan.pageCount).map { scan.imageOfPage(at: $0) })
    }

    func documentCameraViewControllerDidCancel(_ controller: VNDocumentCameraViewController) {
        done([])
    }

    /// 失敗の中身はページの写りに関わるので出さない(やめたのと同じ扱い)。
    func documentCameraViewController(_ controller: VNDocumentCameraViewController, didFailWithError error: Error) {
        done([])
    }
}
#endif

// MARK: - 偽物

/// テスト・プレビュー用。使えるかどうかと返すページを外から決められる。
@MainActor
public final class FakeDocumentScanner: DocumentScanning {
    public var isSupported: Bool
    /// スキャンの画面が返すページの画像(撮った順)
    public var pages: [PlatformImage]
    /// スキャンを出した回数
    public private(set) var scanCount = 0

    public init(isSupported: Bool = true, pages: [PlatformImage] = []) {
        self.isSupported = isSupported
        self.pages = pages
    }

    public func scan() async -> [PlatformImage] {
        scanCount += 1
        return pages
    }
}
