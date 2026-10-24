import SwiftUI
import MetamonDSL

/// `button`: style(primary|secondary|destructive|plain)。押下で `action` をランタイムに渡す。
struct ButtonLeaf: View {
    @Environment(\.dslRuntime) private var rt
    let c: Component
    let ctx: RenderContext

    var body: some View {
        styled(Button {
            guard let action = c.action else { return }
            Task { await rt.run(action, ctx.actionContext()) }
        } label: {
            Group {
                if let icon = c.icon { Label(c.label ?? "", systemImage: icon) } else { Text(c.label ?? "") }
            }
            .frame(maxWidth: c.style == "plain" ? nil : .infinity)
        })
    }

    @ViewBuilder
    private func styled<V: View>(_ button: V) -> some View {
        switch c.style {
        case "secondary": button.buttonStyle(.bordered).controlSize(.large)
        case "destructive": button.buttonStyle(.bordered).controlSize(.large).tint(.red)
        case "plain": button.buttonStyle(.plain)
        default: button.buttonStyle(.borderedProminent).controlSize(.large)
        }
    }
}
