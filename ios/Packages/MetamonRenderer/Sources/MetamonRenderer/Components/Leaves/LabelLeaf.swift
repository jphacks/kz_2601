import SwiftUI
import MetamonDSL

/// `label`: SF Symbol + テキストの1行(SwiftUI Label)。style は text と同じ(title|headline|body|caption)。
struct LabelLeaf: View {
    let c: Component

    var body: some View {
        Label(c.text ?? "", systemImage: c.icon ?? "circle")
            .font(font)
            .frame(maxWidth: .infinity, alignment: .leading)
    }

    private var font: Font {
        switch c.style { case "title": .title.bold(); case "headline": .headline; case "caption": .caption; default: .body }
    }
}
