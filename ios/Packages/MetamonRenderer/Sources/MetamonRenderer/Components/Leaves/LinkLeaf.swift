import SwiftUI
import MetamonDSL

/// `link`: https URL を Safari で開くテキストリンク(SwiftUI Link)。URL が不正なら素のテキスト。
struct LinkLeaf: View {
    let c: Component

    var body: some View {
        Group {
            if let url = c.url.flatMap(URL.init(string:)) {
                Link(destination: url) {
                    Label(c.text ?? url.absoluteString, systemImage: "arrow.up.right.square")
                }
            } else {
                Text(c.text ?? "").foregroundStyle(.secondary)
            }
        }
        .font(.body)
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}
