import SwiftUI
import MetamonDSL

/// `text`: style(title|headline|body|caption)と align(leading|center|trailing)。
/// screen.entity の画面では `{{fieldId}}` を開いているレコードの値にする(qrCode と同じ `QRCodeContent.resolve`)。
/// `highlight` の語に印(marker: 背景 + 太字 / bold)。印は見た目だけで、VoiceOver はふつうの文として読む。
struct TextLeaf: View {
    @Environment(\.dslRuntime) private var rt
    let c: Component
    var ctx: RenderContext? = nil

    var body: some View {
        Text(content)
            .font(font)
            .multilineTextAlignment(alignment)
            .frame(maxWidth: .infinity, alignment: frameAlignment)
    }

    private var content: AttributedString {
        let form = ctx?.form
        let text = QRCodeContent.resolve(c, form: form) { rt.evaluator() }
        guard let h = c.highlight else { return AttributedString(text) }
        let fieldValue = h.field.flatMap { form?.values[$0]?.displayString }
        return TextHighlightStyle.attributed(text, words: h.resolvedWords(fieldValue: fieldValue), style: h.style)
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

enum TextHighlightStyle {
    /// 蛍光ペンの色。ライト / ダークのどちらでも文字色(primary)が読める薄さ。
    static let marker = Color.yellow.opacity(0.4)

    static func attributed(_ text: String, words: [String], style: String?) -> AttributedString {
        var out = AttributedString(text)
        for r in TextHighlightMatcher.ranges(in: text, words: words) {
            guard let lo = AttributedString.Index(r.lowerBound, within: out),
                  let hi = AttributedString.Index(r.upperBound, within: out) else { continue }
            out[lo..<hi].inlinePresentationIntent = .stronglyEmphasized
            if style != "bold" { out[lo..<hi].backgroundColor = marker }
        }
        return out
    }
}
