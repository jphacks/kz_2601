import SwiftUI
import MetamonDSL

/// `button`: style(primary|secondary|destructive|plain)。押下で `action` をランタイムに渡す。
/// primary = ブランドグラデーション + 影 / secondary = ブランドの薄塗り / destructive = red の薄塗り、の 3 つが同じ高さ(52)・角丸(14)で並ぶ(#89)。
/// `deleteRecord` のボタンは編集モード(フォームに recordId がある時)だけ描く(#73: 新規フォームで押しても何も起きない)。
struct ButtonLeaf: View {
    @Environment(\.dslRuntime) private var rt
    let c: Component
    let ctx: RenderContext

    /// 新規フォーム(recordId 無し)では `deleteRecord` を隠す。削除対象が無いので押しても何も起きないため。
    static func isVisible(action: Action?, isEditing: Bool) -> Bool {
        action?.type != .deleteRecord || isEditing
    }

    var body: some View {
        if Self.isVisible(action: c.action, isEditing: ctx.form?.isEditing ?? false) {
            styled
        }
    }

    @ViewBuilder
    private var styled: some View {
        let button = Button {
            guard let action = c.action else { return }
            Task { await rt.run(action, ctx.actionContext()) }
        } label: {
            if let icon = c.icon { Label(c.label ?? "", systemImage: icon) } else { Text(c.label ?? "") }
        }
        switch c.style {
        case "secondary": button.buttonStyle(RenderButtonStyle(kind: .secondary))
        case "destructive": button.buttonStyle(RenderButtonStyle(kind: .destructive))
        case "plain": button.buttonStyle(RenderButtonStyle(kind: .plain))
        default: button.buttonStyle(RenderButtonStyle(kind: .primary))
        }
    }
}

/// Renderer のボタン様式(トークンで高さ・角丸・押下フィードバックを統一)。
/// 高さ 52(plain は 44)は `@ScaledMetric` で Dynamic Type に追従(アクセシビリティサイズでは 2 行まで折り返す)。
struct RenderButtonStyle: ButtonStyle {
    enum Kind { case primary, secondary, destructive, plain }
    let kind: Kind
    @Environment(\.isEnabled) private var isEnabled
    @Environment(\.dynamicTypeSize) private var typeSize
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @ScaledMetric(relativeTo: .body) private var minHeight: CGFloat = 52

    private var shape: RoundedRectangle { RoundedRectangle(cornerRadius: RenderTheme.Radius.button, style: .continuous) }

    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(RenderTheme.Fonts.button)
            .lineLimit(typeSize.isAccessibilitySize ? 2 : 1)
            .minimumScaleFactor(0.8)
            .multilineTextAlignment(.center)
            .padding(.horizontal, RenderTheme.Spacing.md)
            .padding(.vertical, RenderTheme.Spacing.xs)
            .frame(maxWidth: .infinity, minHeight: kind == .plain ? minHeight * 44 / 52 : minHeight)
            .foregroundStyle(foreground)
            .background { background }
            .overlay {
                if kind == .secondary || kind == .destructive {
                    shape.strokeBorder(stroke)
                }
            }
            .shadow(color: kind == .primary && isEnabled ? RenderTheme.Shadow.brandColor : .clear,
                    radius: RenderTheme.Shadow.brandRadius, y: RenderTheme.Shadow.brandY)
            .opacity(isEnabled ? (configuration.isPressed ? 0.9 : 1) : 0.45)
            .scaleEffect(configuration.isPressed && !reduceMotion ? RenderTheme.Motion.pressScale : 1)
            .animation(reduceMotion ? nil : RenderTheme.Motion.press, value: configuration.isPressed)
            .contentShape(shape)
    }

    private var foreground: Color {
        switch kind {
        case .primary: .white
        case .secondary, .plain: RenderTheme.Colors.brandText
        case .destructive: RenderTheme.Colors.negative
        }
    }

    @ViewBuilder
    private var background: some View {
        switch kind {
        case .primary:
            shape.fill(RenderTheme.brandFill)
                .overlay(shape.strokeBorder(.white.opacity(0.18)))
        case .secondary: shape.fill(RenderTheme.Colors.brandSoft)
        case .destructive: shape.fill(RenderTheme.Colors.negative.opacity(0.10))
        case .plain: Color.clear
        }
    }

    private var stroke: Color {
        switch kind {
        case .secondary: RenderTheme.Colors.brandSoftStroke
        case .destructive: RenderTheme.Colors.negative.opacity(0.18)
        default: .clear
        }
    }
}
