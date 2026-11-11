import SwiftUI
import MetamonDSL

/// `link`: URL を Safari / 対応アプリ(Mac は既定のブラウザ)で開くテキストリンク(SwiftUI Link)。
/// `ExternalURLPolicy` で許可されない URL は素のテキスト。Mac はホバーで行き先を出す(#941)。
struct LinkLeaf: View {
    let c: Component

    var body: some View {
        Group {
            if let url = ExternalURLPolicy.openable(c.url) {
                Link(destination: url) {
                    Label(c.text ?? url.absoluteString, systemImage: "arrow.up.right.square")
                        .foregroundStyle(RenderTheme.Colors.brandText)
                }
                .help(url.absoluteString)
                .accessibilityHint(L("ブラウザで開きます"))
            } else {
                Text(c.text ?? "").foregroundStyle(.secondary).macTextSelection()
            }
        }
        .font(.body.weight(.medium))
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}
