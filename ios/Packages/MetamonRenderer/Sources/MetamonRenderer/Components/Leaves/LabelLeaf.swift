import SwiftUI
import MetamonDSL

/// `label`: SF Symbol + テキストの1行(SwiftUI Label)。style は text と同じ(title|headline|body|caption)。
/// Mac に無い記号は `circle` に落とし、文字は選択してコピーできるようにする(#941)。
struct LabelLeaf: View {
    let c: Component

    var body: some View {
        Label(c.text ?? "", systemImage: FieldIcon.resolve(c.icon, fallback: "circle"))
            .font(font)
            .frame(maxWidth: .infinity, alignment: .leading)
            .macTextSelection()
    }

    private var font: Font {
        switch c.style { case "title": .title.bold(); case "headline": .headline; case "caption": .caption; default: .body }
    }
}
