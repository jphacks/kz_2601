import SwiftUI
import MetamonDSL

/// `openUrlButton`: 固定 URL か `app`(+ `to`)を `openURL` で開く(Safari / 対応アプリ)。`style`: primary|secondary|plain(既定 secondary)。
/// url で開けるのは https と `ExternalURLPolicy` の allowlist(line/slack/discord/tel/mailto/maps/comgooglemaps)だけ。
/// app は `OpenApp` が公開されている形のリンクを作る。`to` の `{{fieldId}}` は開いているレコードの値(text と同じ決まり・hidden は入れない)。
/// 候補を先頭から開き、開けなければ次(slack:// が無い端末は Slack の Web)。
struct OpenUrlButtonLeaf: View {
    @Environment(\.openURL) private var openURL
    @Environment(\.dslRuntime) private var rt
    let c: Component
    var ctx: RenderContext? = nil

    var body: some View {
        if let app = OpenApp(c.app) {
            let urls = app.urls(to: c.to.map { TextContent.resolve($0, form: ctx?.form) { rt.evaluator() } })
            styled(Button {
                open(urls[...])
            } label: {
                buttonLabel(c.label ?? app.defaultLabel, icon: c.icon ?? app.defaultIcon)
            })
            .disabled(urls.isEmpty) // phone で番号がまだ無い
        } else if c.app == nil, let url = ExternalURLPolicy.openable(c.url) {
            styled(Button {
                openURL(url)
            } label: {
                buttonLabel(c.label ?? url.absoluteString, icon: c.icon ?? "safari")
            })
        } else {
            UnsupportedComponent(label: L("openUrlButton: url は https か line/slack/discord/tel/mailto/maps のみ"))
        }
    }

    private func open(_ urls: ArraySlice<URL>) { Self.open(urls, with: openURL) }

    private static func open(_ urls: ArraySlice<URL>, with action: OpenURLAction) {
        guard let first = urls.first else { return }
        action(first) { accepted in
            if !accepted { open(urls.dropFirst(), with: action) }
        }
    }

    private func buttonLabel(_ text: String, icon: String) -> some View {
        Label(text, systemImage: icon).frame(maxWidth: c.style == "plain" ? nil : .infinity)
    }

    @ViewBuilder
    private func styled<V: View>(_ button: V) -> some View {
        switch c.style {
        case "primary": button.buttonStyle(RenderButtonStyle(kind: .primary))
        case "plain": button.buttonStyle(RenderButtonStyle(kind: .plain))
        default: button.buttonStyle(RenderButtonStyle(kind: .secondary))
        }
    }
}
