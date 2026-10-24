import SwiftUI
import UIKit
import MetamonDSL

/// schema `SemanticColor` の red/green/orange/blue を、本文サイズの文字色として使っても
/// 面(`RenderTheme.Colors.surface`)と WCAG AA(4.5:1)を満たすトーンに寄せたパレット(Issue #68)。
/// システムの `.red/.green/.orange/.blue` はライトで 2.2〜4.0:1 しか無く、badge の文字・icon が読みにくい。
/// 値は `Tests/SemanticColorContrastTests` がライト(白)/ダーク(#1C1C1E)の面と badge のピル(15% 薄塗り)に対して検算する。
enum SemanticPalette {
    struct RGB: Equatable {
        let r: Double, g: Double, b: Double
        init(_ r: Int, _ g: Int, _ b: Int) { self.r = Double(r) / 255; self.g = Double(g) / 255; self.b = Double(b) / 255 }

        /// WCAG 相対輝度
        var luminance: Double {
            func lin(_ v: Double) -> Double { v <= 0.03928 ? v / 12.92 : pow((v + 0.055) / 1.055, 2.4) }
            return 0.2126 * lin(r) + 0.7152 * lin(g) + 0.0722 * lin(b)
        }

        /// WCAG コントラスト比(1〜21)
        func contrast(with other: RGB) -> Double {
            let (hi, lo) = (max(luminance, other.luminance), min(luminance, other.luminance))
            return (hi + 0.05) / (lo + 0.05)
        }

        /// `self` を `alpha` で `base` に重ねた色(badge のピル背景の実効色)
        func blended(over base: RGB, alpha: Double) -> RGB {
            RGB(Int((r * alpha + base.r * (1 - alpha)) * 255 + 0.5),
                Int((g * alpha + base.g * (1 - alpha)) * 255 + 0.5),
                Int((b * alpha + base.b * (1 - alpha)) * 255 + 0.5))
        }

        /// この色を背景にした時に読める文字色(黒 or 白)。輝度 0.179 を境に切り替える(白とのコントラストが 4.5 を割る点)。
        var readableForeground: Color { luminance > 0.179 ? .black : .white }
    }

    /// (ライト, ダーク)。ライトは白い面に対し 6.2〜7.0:1、ダークは #1C1C1E に対し 6.0〜9.0:1。
    static let red = (light: RGB(190, 28, 22), dark: RGB(255, 105, 97))
    static let green = (light: RGB(16, 112, 48), dark: RGB(72, 214, 110))
    static let orange = (light: RGB(150, 76, 0), dark: RGB(255, 170, 60))
    static let blue = (light: RGB(0, 84, 190), dark: RGB(90, 160, 255))

    /// 検算用: 面の色(secondarySystemGroupedBackground のライト/ダーク)
    static let surfaceLight = RGB(255, 255, 255)
    static let surfaceDark = RGB(28, 28, 30)
    /// badge のピル背景の不透明度
    static let badgeFill: Double = 0.15

    static let all: [(name: String, light: RGB, dark: RGB)] = [
        ("red", red.light, red.dark), ("green", green.light, green.dark),
        ("orange", orange.light, orange.dark), ("blue", blue.light, blue.dark),
    ]

    static func color(_ pair: (light: RGB, dark: RGB)) -> Color {
        Color(UIColor { trait in
            let c = trait.userInterfaceStyle == .dark ? pair.dark : pair.light
            return UIColor(red: c.r, green: c.g, blue: c.b, alpha: 1)
        })
    }
}

extension Color {
    /// schema `SemanticColor`(primary|secondary|accent|red|green|orange|blue)→ Color。未知は primary。
    static func semantic(_ name: String?) -> Color {
        switch name {
        case "secondary": .secondary
        case "accent": .accentColor
        case "red": SemanticPalette.color(SemanticPalette.red)
        case "green": SemanticPalette.color(SemanticPalette.green)
        case "orange": SemanticPalette.color(SemanticPalette.orange)
        case "blue": SemanticPalette.color(SemanticPalette.blue)
        default: .primary
        }
    }
}

extension Component {
    /// icon / avatar の `size`(small|medium|large)→ pt。
    var symbolPointSize: CGFloat {
        switch size { case "small": 16; case "large": 44; default: 28 }
    }
}
