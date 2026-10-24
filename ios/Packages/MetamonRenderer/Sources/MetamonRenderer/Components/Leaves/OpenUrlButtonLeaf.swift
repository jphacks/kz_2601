import SwiftUI
import MetamonDSL

/// `openUrlButton`: 固定 URL を `openURL` で開く(Safari / 対応アプリ)。`style`: primary|secondary|plain(既定 secondary)。
/// 開けるのは https と `ExternalURLPolicy` の allowlist(line/slack/discord/tel/mailto/maps/comgooglemaps)だけ。
struct OpenUrlButtonLeaf: View {
    @Environment(\.openURL) private var openURL
    let c: Component

    var body: some View {
        if let url = ExternalURLPolicy.openable(c.url) {
            styled(Button {
                openURL(url)
            } label: {
                Label(c.label ?? url.absoluteString, systemImage: c.icon ?? "safari")
                    .frame(maxWidth: c.style == "plain" ? nil : .infinity)
            })
        } else {
            UnsupportedComponent(label: "openUrlButton: url は https か line/slack/discord/tel/mailto/maps のみ")
        }
    }

    @ViewBuilder
    private func styled<V: View>(_ button: V) -> some View {
        switch c.style {
        case "primary": button.buttonStyle(.borderedProminent).controlSize(.large)
        case "plain": button.buttonStyle(.plain).foregroundStyle(.tint)
        default: button.buttonStyle(.bordered).controlSize(.large)
        }
    }
}
