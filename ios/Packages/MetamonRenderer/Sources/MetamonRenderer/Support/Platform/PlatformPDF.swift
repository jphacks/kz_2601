import SwiftUI
#if canImport(UIKit)
import UIKit
#else
import AppKit
#endif

// PDF の書き出し(#421)の iOS / Mac の差(mac-002・mac-031)。iOS は UIGraphicsPDFRenderer のまま。

#if canImport(UIKit)
typealias PlatformFont = UIFont
typealias PlatformStringDrawingOptions = NSStringDrawingOptions
#else
typealias PlatformFont = NSFont
typealias PlatformStringDrawingOptions = NSString.DrawingOptions

extension NSFont {
    /// UIFont と同じ名前。1 行の高さ。
    var lineHeight: CGFloat { ceil(ascender - descender + leading) }
}
#endif

enum PlatformPDF {
    /// 原点が左上(UIKit と同じ向き)の PDF。`body` の引数を呼ぶと新しいページ。
    static func data(page: CGRect, title: String, body: (_ beginPage: () -> Void) -> Void) -> Data {
        #if canImport(UIKit)
        let f = UIGraphicsPDFRendererFormat()
        f.documentInfo = [kCGPDFContextTitle as String: title, kCGPDFContextCreator as String: "Tailor It"]
        return UIGraphicsPDFRenderer(bounds: page, format: f).pdfData { ctx in
            body { ctx.beginPage() }
        }
        #else
        let out = NSMutableData()
        var box = page
        let info = [kCGPDFContextTitle as String: title, kCGPDFContextCreator as String: "Tailor It"] as CFDictionary
        guard let consumer = CGDataConsumer(data: out as CFMutableData),
              let cg = CGContext(consumer: consumer, mediaBox: &box, info) else { return Data() }
        var open = false
        NSGraphicsContext.saveGraphicsState()
        NSGraphicsContext.current = NSGraphicsContext(cgContext: cg, flipped: true)
        body {
            if open {
                cg.restoreGState()
                cg.endPDFPage()
            }
            cg.beginPDFPage(nil)
            cg.saveGState()
            cg.translateBy(x: 0, y: page.height)
            cg.scaleBy(x: 1, y: -1)
            open = true
        }
        if open {
            cg.restoreGState()
            cg.endPDFPage()
        }
        NSGraphicsContext.restoreGraphicsState()
        cg.closePDF()
        return out as Data
        #endif
    }

    /// 今の描画先の矩形を今の塗りの色で塗る(UIRectFill)。
    static func fill(_ rect: CGRect) {
        #if canImport(UIKit)
        UIRectFill(rect)
        #else
        rect.fill()
        #endif
    }

    /// ライトの見た目で解決した色(PDF は白地に描く)。
    static func light(_ color: PlatformColor) -> PlatformColor {
        #if canImport(UIKit)
        color.resolvedColor(with: .init(userInterfaceStyle: .light))
        #else
        var resolved = color
        (NSAppearance(named: .aqua) ?? NSAppearance.currentDrawing()).performAsCurrentDrawingAppearance {
            resolved = color.usingColorSpace(.sRGB) ?? color
        }
        return resolved
        #endif
    }

    static var labelColor: PlatformColor {
        #if canImport(UIKit)
        .label
        #else
        .labelColor
        #endif
    }

    static var secondaryLabelColor: PlatformColor {
        #if canImport(UIKit)
        .secondaryLabel
        #else
        .secondaryLabelColor
        #endif
    }

    static var headerFillColor: PlatformColor {
        #if canImport(UIKit)
        .secondarySystemBackground
        #else
        .controlBackgroundColor
        #endif
    }

    static var separatorColor: PlatformColor {
        #if canImport(UIKit)
        .separator
        #else
        .separatorColor
        #endif
    }
}

extension ImageRenderer {
    /// iOS: `uiImage`・Mac: `nsImage`。
    @MainActor var platformImage: PlatformImage? {
        #if canImport(UIKit)
        uiImage
        #else
        nsImage
        #endif
    }
}
