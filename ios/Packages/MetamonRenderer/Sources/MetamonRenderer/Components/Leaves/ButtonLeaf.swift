import SwiftUI
import MetamonDSL

/// `button`: style(primary|secondary|destructive|plain)。押下で `action` をランタイムに渡す。
/// primary = tint 塗り / secondary = tint の薄塗り / destructive = red の薄塗り、の 3 つが同じ高さ・角丸で並ぶ。
struct ButtonLeaf: View {
    @Environment(\.dslRuntime) private var rt
    let c: Component
    let ctx: RenderContext

    var body: some View {
        let button = Button {
            guard let action = c.action else { return }
            Task { await rt.run(action, ctx.actionContext()) }
        } label: {
            if let icon = c.icon { Label(c.label ?? "", systemImage: icon) } else { Text(c.label ?? "") }
        }
        switch c.style {
        case "secondary": button.buttonStyle(RenderButtonStyle(kind: .secondary))
        case "destructive": button.buttonStyle(RenderButtonStyle(kind: .destructive))
        case "plain": button.buttonStyle(.plain).foregroundStyle(.tint)
        default: button.buttonStyle(RenderButtonStyle(kind: .primary))
        }
    }
}

/// Renderer の 3 ボタン様式(トークンで高さ・角丸・押下フィードバックを統一)。
struct RenderButtonStyle: ButtonStyle {
    enum Kind { case primary, secondary, destructive }
    let kind: Kind
    @Environment(\.isEnabled) private var isEnabled

    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(RenderTheme.Fonts.button)
            .lineLimit(1)
            .minimumScaleFactor(0.8)
            .padding(.horizontal, RenderTheme.Spacing.md)
            .frame(maxWidth: .infinity, minHeight: 48)
            .foregroundStyle(foreground)
            .background(background, in: RoundedRectangle(cornerRadius: RenderTheme.Radius.card, style: .continuous))
            .overlay {
                if kind != .primary {
                    RoundedRectangle(cornerRadius: RenderTheme.Radius.card, style: .continuous)
                        .strokeBorder(stroke)
                }
            }
            .opacity(isEnabled ? (configuration.isPressed ? 0.85 : 1) : 0.45)
            .scaleEffect(configuration.isPressed ? 0.98 : 1)
            .animation(.easeOut(duration: 0.12), value: configuration.isPressed)
            .contentShape(RoundedRectangle(cornerRadius: RenderTheme.Radius.card, style: .continuous))
    }

    private var foreground: Color {
        switch kind {
        case .primary: .white
        case .secondary: .accentColor
        case .destructive: RenderTheme.Colors.negative
        }
    }

    private var background: Color {
        switch kind {
        case .primary: .accentColor
        case .secondary: Color.accentColor.opacity(0.12)
        case .destructive: RenderTheme.Colors.negative.opacity(0.12)
        }
    }

    private var stroke: Color {
        switch kind {
        case .primary: .clear
        case .secondary: Color.accentColor.opacity(0.18)
        case .destructive: RenderTheme.Colors.negative.opacity(0.18)
        }
    }
}
