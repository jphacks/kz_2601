import SwiftUI
import MetamonDSL

/// `openUrlButton`: 固定 URL を `openURL` で開く(Safari / 対応アプリ)。`style`: primary|secondary|plain(既定 secondary)。
struct OpenUrlButtonLeaf: View {
    @Environment(\.openURL) private var openURL
    let c: Component

    var body: some View {
        if let s = c.url, let url = URL(string: s) {
            styled(Button {
                openURL(url)
            } label: {
                Label(c.label ?? s, systemImage: c.icon ?? "safari")
                    .frame(maxWidth: c.style == "plain" ? nil : .infinity)
            })
        } else {
            UnsupportedComponent(label: "openUrlButton: url が不正です")
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
