import SwiftUI
import UIKit
import MetamonDSL

/// schema `SemanticColor` の red/green/orange/blue を、本文サイズの文字色として使っても
/// 面(`RenderTheme.Colors.surface`)と WCAG AA(4.5:1)を満たすトーンに寄せたパレット(Issue #68)。
/// システムの `.red/.green/.orange/.blue` はライトで 2.2〜4.0:1 しか無く、badge の文字・icon が読みにくい。
/// 値は `Tests/SemanticColorContrastTests` がライト(#FFFDF8)/ダーク(#16232D)の面と badge のピル(15% 薄塗り)に対して検算する。
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

    /// (ライト, ダーク)。ライトは面 #FFFDF8 に対し 6.0〜6.9:1、ダークは面 #16232D に対し 5.7〜8.5:1。
    static let red = (light: RGB(190, 28, 22), dark: RGB(255, 105, 97))
    static let green = (light: RGB(16, 112, 48), dark: RGB(72, 214, 110))
    static let orange = (light: RGB(150, 76, 0), dark: RGB(255, 170, 60))
    static let blue = (light: RGB(0, 84, 190), dark: RGB(90, 160, 255))
    // MARK: ブランド(#102: アプリアイコンの紺 #335267 × ツルの生成り #F4F1E8 に合わせた「藍 × 生成り」)
    /// 操作色(tint・フォーカスリング・選択中・アイコン)。アイコンの紺を少し明るく・彩度を上げた藍。
    /// ダークでは面 #16232D に沈むので一段明るい藍に切り替える(非文字の 3:1 以上)
    static let brandA = (light: RGB(46, 106, 147), dark: RGB(111, 168, 207))
    /// グラデーションの終点(青緑)。装飾専用(白文字は載せない)
    static let brandB = (light: RGB(62, 154, 156), dark: RGB(95, 192, 190))
    /// ブランドの文字色(リンク・secondary ボタン・値)。面に対し AA(4.5:1)
    static let brand = (light: RGB(31, 90, 130), dark: RGB(140, 195, 230))
    /// 白文字を載せる塗り(primary ボタン・選択チップ・選択日)のグラデーション端点。
    /// 装飾用の brandA→brandB(青緑 #3E9A9C は白文字 3.3:1 どまり)より深い藍→深い青緑で、両端とも白文字 4.5:1 以上
    static let brandFillStart = RGB(36, 94, 134)
    static let brandFillEnd = RGB(40, 111, 126)

    // MARK: 面(ライト = 生成り寄り / ダーク = 紺寄り)
    /// 画面背景。ライトはツルの生成りをごく薄くした色、ダークはアイコンの紺を沈めた濃色
    static let canvas = (light: RGB(246, 243, 236), dark: RGB(15, 26, 34))
    /// カード
    static let surface = (light: RGB(255, 253, 248), dark: RGB(22, 35, 45))
    /// 入力の器(カードの中で一段沈む / ダークでは一段浮く)
    static let surfaceElevated = (light: RGB(249, 246, 239), dark: RGB(30, 46, 58))

    /// chart / カテゴリ色(先頭 2 色はブランド)。アイコンのメジャーの金・とさかの朱・くちばしの砂色 + 同系の藍鼠・灰緑・紺桔梗
    static let chart: [(light: RGB, dark: RGB)] = [
        brandA, brandB,
        (light: RGB(201, 154, 34), dark: RGB(232, 192, 84)),   // 金(メジャー)
        (light: RGB(181, 82, 63), dark: RGB(232, 128, 106)),   // 朱(とさか・糸)
        (light: RGB(108, 139, 163), dark: RGB(150, 180, 204)), // 藍鼠
        (light: RGB(111, 150, 110), dark: RGB(150, 196, 146)), // 灰緑
        (light: RGB(168, 139, 94), dark: RGB(206, 178, 130)),  // 砂色(くちばし)
        (light: RGB(84, 98, 158), dark: RGB(140, 154, 214)),   // 紺桔梗
    ]

    /// 検算用: 面の色(`surface` のライト/ダーク)
    static let surfaceLight = surface.light
    static let surfaceDark = surface.dark
    /// badge のピル背景の不透明度
    static let badgeFill: Double = 0.15

    static let all: [(name: String, light: RGB, dark: RGB)] = [
        ("red", red.light, red.dark), ("green", green.light, green.dark),
        ("orange", orange.light, orange.dark), ("blue", blue.light, blue.dark),
        ("brand", brand.light, brand.dark),
    ]

    static func swiftUI(_ c: RGB) -> Color { Color(red: c.r, green: c.g, blue: c.b) }

    static func color(_ pair: (light: RGB, dark: RGB)) -> Color {
        color(pair, lightAlpha: 1, darkAlpha: 1)
    }

    static func color(_ pair: (light: RGB, dark: RGB), lightAlpha: Double, darkAlpha: Double) -> Color {
        Color(UIColor { trait in
            let dark = trait.userInterfaceStyle == .dark
            let c = dark ? pair.dark : pair.light
            return UIColor(red: c.r, green: c.g, blue: c.b, alpha: dark ? darkAlpha : lightAlpha)
        })
    }
}

extension Color {
    /// schema `SemanticColor`(primary|secondary|accent|red|green|orange|blue)→ Color。未知は primary。
    static func semantic(_ name: String?) -> Color {
        switch name {
        case "secondary": .secondary
        case "accent": RenderTheme.Colors.brandText
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
