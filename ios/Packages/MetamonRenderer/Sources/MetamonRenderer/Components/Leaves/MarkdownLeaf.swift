import SwiftUI
import MetamonDSL

/// `markdown`: インライン Markdown(**太字** / *斜体* / `code` / [link](url))。解釈に失敗したら生テキスト。
struct MarkdownLeaf: View {
    let c: Component

    var body: some View {
        Text(attributed)
            .frame(maxWidth: .infinity, alignment: .leading)
    }

    private var attributed: AttributedString {
        let raw = c.text ?? ""
        let options = AttributedString.MarkdownParsingOptions(interpretedSyntax: .inlineOnlyPreservingWhitespace)
        return (try? AttributedString(markdown: raw, options: options)) ?? AttributedString(raw)
    }
}
