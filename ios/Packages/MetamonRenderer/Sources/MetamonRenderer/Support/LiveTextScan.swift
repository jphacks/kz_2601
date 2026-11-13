import Foundation
#if canImport(UIKit)
import UIKit
#else
import AppKit
#endif
#if canImport(VisionKit) && canImport(UIKit)
import VisionKit
#endif
#if canImport(AVFoundation)
import AVFoundation
#endif
import MetamonDSL

/// カメラを向けて文字・数字を読む口(#2228・親 #1827)。
/// `DataScannerViewController` で読み、タップした 1 つを返す。
/// 部品・式・AI の決まり・`supportedFeatures` の申告はまだ無い(#1775 の決定待ち)。
/// `DocumentScan` / `ComposeMessage` と同じ形: protocol + 実体(`LiveTextScanner`)+ 偽物(`FakeLiveTextScanner`)。
///
/// 画面(`DataScannerViewController`)は単体テストできないので、
/// 「読んだ文字 → 数字」「読んだ行 → 欲しい 1 つ」は純関数に分けてある(Mac でもテストできる)。
///
/// 読んだ中身は端末の中だけで扱う。ファイルにも保存せず、ログにも出さない。

// MARK: - 何を読むか

/// 読む対象。`DataScannerViewController` は「数字だけ」を直接は読めないので、
/// `.number` は文字として読んだうえで `ScannedNumber` で数を取り出す。
public enum LiveTextScanKind: String, Sendable, CaseIterable {
    case text
    case number
    case phone
    case email
    case url
}

// MARK: - 文字 → 数字(純関数・単体テストの対象)

/// 読んだ文字から数を取り出す(純関数・テスト対象)。
///
/// 決めごと:
/// - 全角の数字・記号は半角に寄せる(「１２３」→ 123)。`ReceiptParser.normalize` と同じ寄せ方を使う
/// - カンマは 3 桁区切りとして落とす(「1,234」→ 1234)。`,` のあとが 3 桁でなければ区切りとは見ない
/// - ピリオドは小数点(「12.5」→ 12.5)。3 桁区切りのピリオドは扱わない(日本語のレシート向け)
/// - 通貨記号(¥ ￥ $ など)・単位(kg 円 %)はただの前後の文字として捨てる(「¥1,234」→ 1234・「12.5kg」→ 12.5)
/// - 数の直前の `-` `−` `▲` `△` はマイナス(「△1,000」→ -1000)。数のうしろの `-` は次の数の区切り(「5-3」→ 5 と 3)
/// - 数が複数あれば出てきた順に全部返す。`number(from:)` は最初の 1 つ
/// - 数が無ければ nil
public enum ScannedNumber {
    private static let regex = try! NSRegularExpression(
        pattern: #"(?<![0-9.])[-−▲△]?\s*(?:[0-9]{1,3}(?:,[0-9]{3})+|[0-9]+)(?:\.[0-9]+)?"#)

    /// 文字の中の数を出てきた順に全部。
    public static func numbers(in raw: String) -> [Double] {
        let text = ReceiptParser.normalize(raw)
        let ns = text as NSString
        return regex.matches(in: text, range: NSRange(location: 0, length: ns.length))
            .compactMap { value(of: ns.substring(with: $0.range)) }
    }

    /// 文字の中の最初の数。数が無ければ nil。
    public static func number(from raw: String) -> Double? { numbers(in: raw).first }

    /// 切り出した 1 つ分(「-1,234.5」など)を数にする。
    private static func value(of piece: String) -> Double? {
        var s = piece.replacingOccurrences(of: ",", with: "")
        s = s.components(separatedBy: .whitespaces).joined()
        var negative = false
        for sign in ["-", "−", "▲", "△"] where s.hasPrefix(sign) {
            negative = true
            s.removeFirst()
            break
        }
        guard let v = Double(s), v.isFinite else { return nil }
        return negative ? -v : v
    }
}

// MARK: - 読んだ行 → 欲しい 1 つ(純関数・単体テストの対象)

/// 写真から読んだ行の並びから、欲しい種類の 1 つを選ぶ(純関数・テスト対象)。
/// カメラで読めない端末のフォールバックで使う。
public enum LiveTextScanPick {
    /// 上の行から順に見て、最初に当てはまったものを返す。無ければ nil。
    /// `.text` は行そのまま、`.number` は数を含む行そのまま(数の取り出しは `ScannedNumber`)、
    /// `.phone` / `.email` / `.url` は行の中の当てはまった部分だけ。
    public static func first(kind: LiveTextScanKind, in lines: [String]) -> String? {
        for raw in lines {
            let line = raw.trimmingCharacters(in: .whitespacesAndNewlines)
            guard !line.isEmpty else { continue }
            switch kind {
            case .text:
                return line
            case .number:
                if ScannedNumber.number(from: line) != nil { return line }
            case .phone:
                if let hit = detected(.phoneNumber, in: line) { return hit }
            case .email:
                if let hit = email(in: line) { return hit }
            case .url:
                if let hit = detected(.link, in: line), !hit.contains("@") { return hit }
            }
        }
        return nil
    }

    private static func detected(_ type: NSTextCheckingResult.CheckingType, in line: String) -> String? {
        guard let detector = try? NSDataDetector(types: type.rawValue) else { return nil }
        let ns = line as NSString
        guard let m = detector.firstMatch(in: line, range: NSRange(location: 0, length: ns.length)) else { return nil }
        return ns.substring(with: m.range)
    }

    private static let emailRegex = try! NSRegularExpression(
        pattern: #"[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}"#)

    private static func email(in line: String) -> String? {
        let ns = line as NSString
        guard let m = emailRegex.firstMatch(in: line, range: NSRange(location: 0, length: ns.length)) else { return nil }
        return ns.substring(with: m.range)
    }
}

// MARK: - 読む口

@MainActor
public protocol LiveTextScanning {
    /// この端末で動くか(A12 以降・iOS 16 以降。Mac・古い端末では false)。
    var isSupported: Bool { get }
    /// 今すぐ使えるか(カメラの許可・ほかのアプリの利用などで変わる)。
    var isAvailable: Bool { get }
    /// 読み取りの画面を出して、1 つタップされるまで待つ。
    /// タップされた文字を返す。やめた時・出せなかった時は nil。
    func scan(kind: LiveTextScanKind) async -> String?
}

/// 使えない端末のフォールバックで使う「1 枚の写真を手に入れる口」。
@MainActor
public protocol LiveTextPhotoSourcing {
    /// 写真を 1 枚。やめた時・撮れない時は nil。
    func photo() async -> PlatformImage?
}

@MainActor
public enum LiveTextScan {
    /// 読む口の実体。テスト・プレビューでは差し替える。
    public static var scanner: any LiveTextScanning = LiveTextScanner()
    /// 使えない端末で写真から読むときの写真の出どころ。テスト・プレビューでは差し替える。
    public static var photoSource: any LiveTextPhotoSourcing = LiveTextCameraPhotoSource()

    public static var isSupported: Bool { scanner.isSupported }
    public static var isAvailable: Bool { scanner.isAvailable }

    /// タップされた文字。使えない端末では写真から読む。どちらも駄目なら nil。
    public static func text(kind: LiveTextScanKind = .text) async -> String? {
        await text(kind: kind, using: scanner, photoSource: photoSource)
    }

    /// タップされた文字から取り出した数。数が無ければ nil。
    public static func number() async -> Double? {
        await number(using: scanner, photoSource: photoSource)
    }

    /// 以下は口を差し替えられる形(単体テストはこちらを呼ぶ)。
    public static func text(kind: LiveTextScanKind,
                            using scanner: any LiveTextScanning,
                            photoSource: any LiveTextPhotoSourcing) async -> String? {
        if scanner.isSupported, scanner.isAvailable,
           let scanned = await scanner.scan(kind: kind), !scanned.isEmpty {
            return scanned
        }
        return await fromPhoto(kind: kind, photoSource: photoSource)
    }

    public static func number(using scanner: any LiveTextScanning,
                              photoSource: any LiveTextPhotoSourcing) async -> Double? {
        guard let text = await text(kind: .number, using: scanner, photoSource: photoSource) else { return nil }
        return ScannedNumber.number(from: text)
    }

    /// 写真 1 枚を Vision の文字認識(`PhotoVision.textLines`)に通して、欲しい 1 つを選ぶ。
    static func fromPhoto(kind: LiveTextScanKind, photoSource: any LiveTextPhotoSourcing) async -> String? {
        guard let image = await photoSource.photo(), let cg = image.cgImage else { return nil }
        let lines = await Task.detached(priority: .userInitiated) { PhotoVision.textLines(in: cg) }.value
        return LiveTextScanPick.first(kind: kind, in: lines)
    }
}

// MARK: - 実体

/// iOS 16 以降・A12 以降では `DataScannerViewController`。それ以外では `isSupported` が false。
@MainActor
public final class LiveTextScanner: LiveTextScanning {
    public init() {}

    public var isSupported: Bool {
        #if canImport(VisionKit) && canImport(UIKit)
        if #available(iOS 16.0, *) { return DataScannerViewController.isSupported }
        return false
        #else
        return false
        #endif
    }

    public var isAvailable: Bool {
        #if canImport(VisionKit) && canImport(UIKit)
        if #available(iOS 16.0, *) { return DataScannerViewController.isAvailable }
        return false
        #else
        return false
        #endif
    }

    public func scan(kind: LiveTextScanKind) async -> String? {
        #if canImport(VisionKit) && canImport(UIKit)
        guard #available(iOS 16.0, *), isSupported else { return nil }
        guard await LiveTextCameraAccess.request() else { return nil }
        guard isAvailable, let host = LiveTextScanner.topViewController() else { return nil }
        let bridge = LiveTextScanDelegateBridge()
        let controller = DataScannerViewController(recognizedDataTypes: Self.dataTypes(for: kind),
                                                   qualityLevel: .balanced,
                                                   recognizesMultipleItems: false,
                                                   isHighFrameRateTrackingEnabled: false,
                                                   isPinchToZoomEnabled: true,
                                                   isGuidanceEnabled: true,
                                                   isHighlightingEnabled: true)
        controller.delegate = bridge
        return await withCheckedContinuation { continuation in
            bridge.finish = { [weak controller] text in
                controller?.stopScanning()
                controller?.dismiss(animated: true)
                continuation.resume(returning: text)
            }
            host.present(controller, animated: true) {
                // 出せても始められないことがある(カメラをほかが使っている等)。その時はやめたのと同じ扱い。
                if (try? controller.startScanning()) == nil { bridge.cancel() }
            }
        }
        #else
        return nil
        #endif
    }

    #if canImport(VisionKit) && canImport(UIKit)
    @available(iOS 16.0, *)
    static func dataTypes(for kind: LiveTextScanKind) -> Set<DataScannerViewController.RecognizedDataType> {
        switch kind {
        case .text, .number:
            // 数字だけを読む指定は無いので、文字として読んで `ScannedNumber` で取り出す。
            return [.text(languages: ["ja-JP", "en-US"])]
        case .phone:
            return [.text(languages: ["ja-JP", "en-US"], textContentType: .telephoneNumber)]
        case .email:
            return [.text(languages: ["ja-JP", "en-US"], textContentType: .emailAddress)]
        case .url:
            return [.text(languages: ["ja-JP", "en-US"], textContentType: .URL)]
        }
    }
    #endif

    #if canImport(UIKit)
    /// 今いちばん上の画面(そこに読み取りの画面を重ねる)。
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

/// カメラの許可。未決定ならダイアログを出して結果を返す。
enum LiveTextCameraAccess {
    static func request() async -> Bool {
        #if canImport(AVFoundation)
        switch AVCaptureDevice.authorizationStatus(for: .video) {
        case .authorized: return true
        case .notDetermined: return await AVCaptureDevice.requestAccess(for: .video)
        default: return false
        }
        #else
        return false
        #endif
    }
}

#if canImport(VisionKit) && canImport(UIKit)
/// `DataScannerViewController` のデリゲートを「タップされた文字 1 つ」にまとめる。
/// 閉じるまで自分で自分を持つ(`delegate` は weak なので)。
@available(iOS 16.0, *)
@MainActor
private final class LiveTextScanDelegateBridge: NSObject, @preconcurrency DataScannerViewControllerDelegate {
    var finish: ((String?) -> Void)?
    private var retain: LiveTextScanDelegateBridge?

    override init() {
        super.init()
        retain = self
    }

    func cancel() { done(nil) }

    private func done(_ text: String?) {
        let finish = self.finish
        self.finish = nil
        retain = nil
        finish?(text)
    }

    func dataScanner(_ scanner: DataScannerViewController, didTapOn item: RecognizedItem) {
        done(Self.text(of: item))
    }

    /// 読み取りが始められなかった時。中身は写っているものに関わるので出さない(やめたのと同じ扱い)。
    func dataScanner(_ scanner: DataScannerViewController, becameUnavailableWithError error: DataScannerViewController.ScanningUnavailable) {
        done(nil)
    }

    static func text(of item: RecognizedItem) -> String? {
        switch item {
        case .text(let text): return text.transcript
        case .barcode(let code): return code.payloadStringValue
        @unknown default: return nil
        }
    }
}
#endif

/// 使えない端末のフォールバックの写真。iOS ではカメラで 1 枚撮る。Mac・カメラの無い端末では nil。
@MainActor
public final class LiveTextCameraPhotoSource: LiveTextPhotoSourcing {
    public init() {}

    public func photo() async -> PlatformImage? {
        #if canImport(UIKit) && os(iOS)
        guard UIImagePickerController.isSourceTypeAvailable(.camera),
              await LiveTextCameraAccess.request(),
              let host = LiveTextScanner.topViewController() else { return nil }
        let bridge = LiveTextPhotoPickerBridge()
        let picker = UIImagePickerController()
        picker.sourceType = .camera
        picker.cameraCaptureMode = .photo
        picker.cameraDevice = .rear
        picker.allowsEditing = false
        picker.delegate = bridge
        return await withCheckedContinuation { continuation in
            bridge.finish = { [weak picker] image in
                picker?.dismiss(animated: true)
                continuation.resume(returning: image)
            }
            host.present(picker, animated: true)
        }
        #else
        return nil
        #endif
    }
}

#if canImport(UIKit) && os(iOS)
/// `UIImagePickerController` のデリゲートを「写真 1 枚」にまとめる。閉じるまで自分で自分を持つ。
@MainActor
private final class LiveTextPhotoPickerBridge: NSObject, @preconcurrency UIImagePickerControllerDelegate, @preconcurrency UINavigationControllerDelegate {
    var finish: ((PlatformImage?) -> Void)?
    private var retain: LiveTextPhotoPickerBridge?

    override init() {
        super.init()
        retain = self
    }

    private func done(_ image: PlatformImage?) {
        let finish = self.finish
        self.finish = nil
        retain = nil
        finish?(image)
    }

    func imagePickerController(_ picker: UIImagePickerController,
                               didFinishPickingMediaWithInfo info: [UIImagePickerController.InfoKey: Any]) {
        done(info[.originalImage] as? PlatformImage)
    }

    func imagePickerControllerDidCancel(_ picker: UIImagePickerController) {
        done(nil)
    }
}
#endif

// MARK: - 偽物

/// テスト・プレビュー用。使えるかどうかと返す文字を外から決められる。
@MainActor
public final class FakeLiveTextScanner: LiveTextScanning {
    public var isSupported: Bool
    public var isAvailable: Bool
    /// 読み取りの画面が返す文字(nil はやめた)
    public var scanned: String?
    /// 読み取りを出した回数
    public private(set) var scanCount = 0
    /// 最後に頼まれた種類
    public private(set) var lastKind: LiveTextScanKind?

    public init(isSupported: Bool = true, isAvailable: Bool = true, scanned: String? = nil) {
        self.isSupported = isSupported
        self.isAvailable = isAvailable
        self.scanned = scanned
    }

    public func scan(kind: LiveTextScanKind) async -> String? {
        scanCount += 1
        lastKind = kind
        return scanned
    }
}

/// テスト・プレビュー用の写真の出どころ。
@MainActor
public final class FakeLiveTextPhotoSource: LiveTextPhotoSourcing {
    public var image: PlatformImage?
    public private(set) var photoCount = 0

    public init(image: PlatformImage? = nil) {
        self.image = image
    }

    public func photo() async -> PlatformImage? {
        photoCount += 1
        return image
    }
}
