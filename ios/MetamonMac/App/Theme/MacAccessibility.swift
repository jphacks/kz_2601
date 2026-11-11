import SwiftUI
import AppKit

/// 「透明度を下げる」「コントラストを上げる」に合わせた面と枠と文字(#1206)。
/// 材質(material)の面は、透明度を下げる時に不透明な theme の色に替える。コントラストを上げる時は枠を濃く・太くし、補足の文字を本文の色にする。
/// サイドバー・ツールバー・シートの system の材質は OS が自分で不透明にするので、ここで扱うのは本体が自分で敷く材質(一言・お知らせ・バー)だけ。
struct MacSurfaceStyle: Equatable {
    enum Fill: Equatable {
        /// 下が透けて見える材質(regularMaterial / bar)
        case material
        /// 不透明な theme の色
        case solid
    }

    enum Border: Equatable {
        /// いつもの細い枠(`.separator`)
        case hairline
        /// 濃く太い枠
        case strong
    }

    enum SecondaryText: Equatable {
        /// `.secondary`
        case secondary
        /// 本文と同じ `.primary`
        case primary
    }

    let fill: Fill
    let border: Border
    let secondaryText: SecondaryText

    static let standard = MacSurfaceStyle(fill: .material, border: .hairline, secondaryText: .secondary)

    static func resolve(reduceTransparency: Bool, contrast: ColorSchemeContrast) -> MacSurfaceStyle {
        let increased = contrast == .increased
        return MacSurfaceStyle(fill: reduceTransparency ? .solid : .material,
                               border: increased ? .strong : .hairline,
                               secondaryText: increased ? .primary : .secondary)
    }

    var borderWidth: CGFloat { border == .strong ? 1.5 : 1 }

    /// 一言・お知らせのカード(浮いている面)
    var raisedFill: AnyShapeStyle {
        fill == .solid ? AnyShapeStyle(Theme.Colors.surfaceElevated) : AnyShapeStyle(.regularMaterial)
    }

    /// 中央の列の上に貼るバー
    var barFill: AnyShapeStyle {
        fill == .solid ? AnyShapeStyle(Theme.Colors.surface) : AnyShapeStyle(.bar)
    }

    var borderStyle: AnyShapeStyle {
        border == .strong ? AnyShapeStyle(MacTheme.Colors.strongHairline) : AnyShapeStyle(.separator)
    }

    /// 枠の色を `RenderTheme` の hairline で描いている所(画面マップのカード)
    var hairlineStyle: AnyShapeStyle {
        border == .strong ? AnyShapeStyle(MacTheme.Colors.strongHairline) : AnyShapeStyle(Theme.Colors.hairline)
    }

    var secondaryTextStyle: AnyShapeStyle {
        secondaryText == .primary ? AnyShapeStyle(.primary) : AnyShapeStyle(.secondary)
    }
}

extension MacTheme {
    enum Colors {
        /// コントラストを上げる時の枠。`RenderTheme.Colors.hairline` と同じ色で不透明度だけ上げる(ライト 紺 / ダーク 生成り)
        static let strongHairline = Color(nsColor: NSColor(name: nil) { appearance in
            appearance.bestMatch(from: [.aqua, .darkAqua]) == .darkAqua
                ? NSColor(srgbRed: 244 / 255, green: 241 / 255, blue: 232 / 255, alpha: 0.55)
                : NSColor(srgbRed: 51 / 255, green: 82 / 255, blue: 103 / 255, alpha: 0.60)
        })
    }
}

private struct MacSurfaceStyleReader<Content: View>: View {
    @Environment(\.accessibilityReduceTransparency) private var reduceTransparency
    @Environment(\.colorSchemeContrast) private var contrast
    let content: (MacSurfaceStyle) -> Content

    var body: some View {
        content(MacSurfaceStyle.resolve(reduceTransparency: reduceTransparency, contrast: contrast))
    }
}

private struct MacRaisedSurface<S: InsettableShape>: ViewModifier {
    let shape: S

    func body(content: Content) -> some View {
        MacSurfaceStyleReader { style in
            content
                .background(style.raisedFill, in: shape)
                .overlay(shape.strokeBorder(style.borderStyle, lineWidth: style.borderWidth))
        }
    }
}

private struct MacBarSurface: ViewModifier {
    func body(content: Content) -> some View {
        MacSurfaceStyleReader { style in
            content
                .background(style.barFill)
                .overlay(alignment: .bottom) {
                    Rectangle().fill(style.borderStyle).frame(height: style.borderWidth)
                }
        }
    }
}

private struct MacSecondaryText: ViewModifier {
    func body(content: Content) -> some View {
        MacSurfaceStyleReader { style in content.foregroundStyle(style.secondaryTextStyle) }
    }
}

extension View {
    /// 一言・お知らせの浮いた面。材質か不透明な色と、枠(#1206)
    func macRaisedSurface<S: InsettableShape>(in shape: S) -> some View {
        modifier(MacRaisedSurface(shape: shape))
    }

    /// 中央の列の上のバー。材質か不透明な色と、下の区切り線(#1206)
    func macBarSurface() -> some View {
        modifier(MacBarSurface())
    }

    /// 補足の文字。コントラストを上げる時は本文と同じ色(#1206)
    func macSecondaryText() -> some View {
        modifier(MacSecondaryText())
    }
}

/// 表示メニューの「拡大」⌘+ /「縮小」⌘- /「実際のサイズ」⌘0(#1206)。中央の生成したアプリの文字を Dynamic Type の段で上げ下げする。
/// 段は UserDefaults に残す(ウインドウ・起動をまたいで同じ)。端の段では押せない。
enum MacTextZoom {
    static let storageKey = LocalWipe.macTextZoomKey

    /// 選べる段。iOS の「さらに大きな文字」の手前(accessibility3)までにする(Mac のウインドウ幅で崩れない所まで)
    static let steps: [DynamicTypeSize] = [
        .xSmall, .small, .medium, .large, .xLarge, .xxLarge, .xxxLarge,
        .accessibility1, .accessibility2, .accessibility3,
    ]

    /// 「実際のサイズ」= .large(system の既定)
    static let defaultStep = steps.firstIndex(of: .large)!

    static func clamped(_ step: Int) -> Int { min(max(step, 0), steps.count - 1) }

    static func size(at step: Int) -> DynamicTypeSize { steps[clamped(step)] }

    static func canZoomIn(_ step: Int) -> Bool { clamped(step) < steps.count - 1 }
    static func canZoomOut(_ step: Int) -> Bool { clamped(step) > 0 }
    static func isActualSize(_ step: Int) -> Bool { clamped(step) == defaultStep }

    static func zoomedIn(_ step: Int) -> Int { clamped(clamped(step) + 1) }
    static func zoomedOut(_ step: Int) -> Int { clamped(clamped(step) - 1) }

    /// 残っている段(無い・範囲外なら既定 / 端に寄せる)
    static func step(in defaults: UserDefaults) -> Int {
        guard defaults.object(forKey: storageKey) != nil else { return defaultStep }
        return clamped(defaults.integer(forKey: storageKey))
    }

    static func setStep(_ step: Int, in defaults: UserDefaults) {
        defaults.set(clamped(step), forKey: storageKey)
    }

    /// 本文(body)の大きさの、実際のサイズ(17pt)に対する割合。設定の選択肢に出す(#937)。
    /// 値は iOS の Dynamic Type の body の pt(xSmall 14 … accessibility3 40)
    static let bodyPoints: [Double] = [14, 15, 16, 17, 19, 21, 23, 28, 33, 40]

    static func percent(at step: Int) -> Int {
        Int((bodyPoints[clamped(step)] / bodyPoints[defaultStep] * 100).rounded())
    }
}

/// 「視差効果を減らす」(#937)。殻の出し入れ・場面の切り替えを、動かさずにその場で変える。
/// View の中は `@Environment(\.accessibilityReduceMotion)`、View の外(会話のセッションなど)は `systemReduceMotion` で読む。
@MainActor
enum MacMotion {
    /// system の設定。テストで差し替える。
    static var systemReduceMotion: () -> Bool = { NSWorkspace.shared.accessibilityDisplayShouldReduceMotion }

    /// 動きを減らす時は nil(アニメーション無しで変える)
    static func animation(_ base: Animation? = .default, reduceMotion: Bool) -> Animation? {
        reduceMotion ? nil : base
    }

    /// 上から滑り込む一言・バーの出し入れ。動きを減らす時はその場で薄く出す
    static func topTransition(reduceMotion: Bool) -> AnyTransition {
        reduceMotion ? .opacity : .move(edge: .top).combined(with: .opacity)
    }
}

/// `withAnimation` の代わり。「視差効果を減らす」の時はアニメーション無しで変える(#937)
@MainActor
func withMacAnimation<Result>(_ animation: Animation? = .default, _ body: () throws -> Result) rethrows -> Result {
    try withAnimation(MacMotion.animation(animation, reduceMotion: MacMotion.systemReduceMotion()), body)
}

private struct MacTopTransition: ViewModifier {
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    func body(content: Content) -> some View {
        content.transition(MacMotion.topTransition(reduceMotion: reduceMotion))
    }
}

private struct MacMotionAnimation<V: Equatable>: ViewModifier {
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    let animation: Animation?
    let value: V

    func body(content: Content) -> some View {
        content.animation(MacMotion.animation(animation, reduceMotion: reduceMotion), value: value)
    }
}

/// 中央の列のようにフォーカスを受ける広い面の輪(#937)。クリックで入った時は出さず、
/// フルキーボードアクセス(システム設定 > キーボード > キーボードナビゲーション)が有効な時だけ描く。
@MainActor
enum MacFocusRing {
    static var fullKeyboardAccess: () -> Bool = { NSApp?.isFullKeyboardAccessEnabled ?? false }

    static func isVisible(focused: Bool, fullKeyboardAccess: Bool) -> Bool { focused && fullKeyboardAccess }
}

private struct MacPaneFocusRing: ViewModifier {
    let focused: Bool

    func body(content: Content) -> some View {
        content.overlay {
            if MacFocusRing.isVisible(focused: focused, fullKeyboardAccess: MacFocusRing.fullKeyboardAccess()) {
                RoundedRectangle(cornerRadius: 6, style: .continuous)
                    .strokeBorder(Color(nsColor: .keyboardFocusIndicatorColor), lineWidth: 3)
                    .padding(2)
                    .allowsHitTesting(false)
                    .accessibilityHidden(true)
            }
        }
    }
}

extension View {
    /// 広い面のフォーカスの輪。`.focusEffectDisabled()` と組にして使う(#937)
    func macPaneFocusRing(_ focused: Bool) -> some View {
        modifier(MacPaneFocusRing(focused: focused))
    }

    /// 一言・バーの出し入れ(上から)。「視差効果を減らす」の時は薄く出すだけ(#937)
    func macTopTransition() -> some View {
        modifier(MacTopTransition())
    }

    /// `.animation(_:value:)` の代わり。「視差効果を減らす」の時は動かさない(#937)
    func macAnimation<V: Equatable>(_ animation: Animation? = .default, value: V) -> some View {
        modifier(MacMotionAnimation(animation: animation, value: value))
    }
}
