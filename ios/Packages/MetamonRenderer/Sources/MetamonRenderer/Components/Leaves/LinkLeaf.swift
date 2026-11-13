import SwiftUI
import MetamonDSL

/// `link`: URL を Safari / 対応アプリで開くテキストリンク(SwiftUI Link)。`ExternalURLPolicy` で許可されない URL は素のテキスト。
struct LinkLeaf: View {
    let c: Component

    var body: some View {
        Group {
            if let url = ExternalURLPolicy.openable(c.url) {
                Link(destination: url) {
                    Label(c.text ?? url.absoluteString, systemImage: "arrow.up.right.square")
                        .foregroundStyle(RenderTheme.Colors.brandText)
                }
            } else {
                Text(c.text ?? "").foregroundStyle(.secondary)
                    .macTextSelection { c.text ?? "" } // Mac: リンクにしない文字は選んでコピーできる(#941)
            }
        }
        .font(.body.weight(.medium))
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}
