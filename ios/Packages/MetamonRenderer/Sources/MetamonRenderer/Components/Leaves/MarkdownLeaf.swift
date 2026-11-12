import SwiftUI
import MetamonDSL

/// `markdown`: インライン Markdown(**太字** / *斜体* / `code` / [link](url))。解釈に失敗したら生テキスト。
/// リンクは `ExternalURLPolicy` が許す URL だけ生かし、それ以外は文字だけ残す(#941)。
struct MarkdownLeaf: View {
    let c: Component

    var body: some View {
        let attributed = Self.attributed(c.text ?? "")
        Text(attributed)
            .macTextSelection { String(attributed.characters) } // Mac: 選んでコピーできる(#941)。リンクは標準の openURL で既定のブラウザ
            .frame(maxWidth: .infinity, alignment: .leading)
    }

    /// 解釈した Markdown。許されないリンク(javascript: や相対パス)はリンクを外す
    static func attributed(_ raw: String) -> AttributedString {
        let options = AttributedString.MarkdownParsingOptions(interpretedSyntax: .inlineOnlyPreservingWhitespace)
        guard var out = try? AttributedString(markdown: raw, options: options) else { return AttributedString(raw) }
        let blocked = out.runs.compactMap { run in
            run.link.flatMap { ExternalURLPolicy.openable($0.absoluteString) == nil ? run.range : nil }
        }
        for range in blocked { out[range].link = nil }
        return out
    }
}
