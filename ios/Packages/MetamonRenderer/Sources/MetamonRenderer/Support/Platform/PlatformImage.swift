import SwiftUI
#if canImport(UIKit)
import UIKit
#else
import AppKit
#endif

// iOS と Mac の画像の差を吸収する(mac-002)。iOS では UIImage そのもので、挙動は変わらない。

#if canImport(UIKit)
public typealias PlatformImage = UIImage
#else
public typealias PlatformImage = NSImage
#endif

extension Image {
    init(platformImage: PlatformImage) {
        #if canImport(UIKit)
        self.init(uiImage: platformImage)
        #else
        self.init(nsImage: platformImage)
        #endif
    }
}

enum PlatformGraphics {
    /// `size`(pt)× `scale` のビットマップに描く。原点は左上(UIKit と同じ向き)。
    /// iOS は UIGraphicsImageRenderer。`draw` の中では `PlatformColor.setFill()` と `PlatformImage.drawUpright(in:)` が使える。
    static func image(size: CGSize, scale: CGFloat, opaque: Bool, draw: (CGContext) -> Void) -> PlatformImage {
        #if canImport(UIKit)
        let format = UIGraphicsImageRendererFormat.default()
        format.scale = scale
        format.opaque = opaque
        return UIGraphicsImageRenderer(size: size, format: format).image { draw($0.cgContext) }
        #else
        let w = max(1, Int((size.width * scale).rounded())), h = max(1, Int((size.height * scale).rounded()))
        let info = opaque ? CGImageAlphaInfo.noneSkipLast.rawValue : CGImageAlphaInfo.premultipliedLast.rawValue
        guard let cg = CGContext(data: nil, width: w, height: h, bitsPerComponent: 8, bytesPerRow: 0,
                                 space: CGColorSpace(name: CGColorSpace.sRGB)!, bitmapInfo: info) else {
            return NSImage(size: size)
        }
        cg.translateBy(x: 0, y: CGFloat(h))
        cg.scaleBy(x: scale, y: -scale)
        NSGraphicsContext.saveGraphicsState()
        NSGraphicsContext.current = NSGraphicsContext(cgContext: cg, flipped: true)
        draw(cg)
        NSGraphicsContext.restoreGraphicsState()
        guard let out = cg.makeImage() else { return NSImage(size: size) }
        return NSImage(cgImage: out, size: size)
        #endif
    }
}

extension PlatformImage {
    /// 上下を正しく描く(Mac の反転した座標でも逆さにしない)。
    func drawUpright(in rect: CGRect) {
        #if canImport(UIKit)
        draw(in: rect)
        #else
        draw(in: rect, from: .zero, operation: .sourceOver, fraction: 1, respectFlipped: true, hints: nil)
        #endif
    }
}

#if !canImport(UIKit)
extension NSImage {
    convenience init(cgImage: CGImage) {
        self.init(cgImage: cgImage, size: NSSize(width: cgImage.width, height: cgImage.height))
    }

    /// UIImage と同じ名前。Mac の NSImage は pt = px として扱う(scale は 1)。
    var cgImage: CGImage? {
        var rect = CGRect(origin: .zero, size: size)
        return cgImage(forProposedRect: &rect, context: nil, hints: nil)
    }

    var scale: CGFloat {
        guard let cg = cgImage, size.width > 0 else { return 1 }
        return CGFloat(cg.width) / size.width
    }

    func jpegData(compressionQuality: CGFloat) -> Data? {
        guard let cg = cgImage else { return nil }
        return NSBitmapImageRep(cgImage: cg).representation(using: .jpeg, properties: [.compressionFactor: compressionQuality])
    }

    func pngData() -> Data? {
        guard let cg = cgImage else { return nil }
        return NSBitmapImageRep(cgImage: cg).representation(using: .png, properties: [:])
    }
}
#endif
