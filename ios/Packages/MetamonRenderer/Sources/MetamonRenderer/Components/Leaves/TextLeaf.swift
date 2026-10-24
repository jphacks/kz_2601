import SwiftUI
import MetamonDSL

/// `text`: style(title|headline|body|caption)と align(leading|center|trailing)。
struct TextLeaf: View {
    let c: Component

    var body: some View {
        Text(c.text ?? "")
            .font(font)
            .multilineTextAlignment(alignment)
            .frame(maxWidth: .infinity, alignment: frameAlignment)
    }

    private var font: Font {
        switch c.style { case "title": .title.bold(); case "headline": .headline; case "caption": .caption; default: .body }
    }
    private var alignment: TextAlignment {
        switch c.align { case "center": .center; case "trailing": .trailing; default: .leading }
    }
    private var frameAlignment: Alignment {
        switch c.align { case "center": .center; case "trailing": .trailing; default: .leading }
    }
}
