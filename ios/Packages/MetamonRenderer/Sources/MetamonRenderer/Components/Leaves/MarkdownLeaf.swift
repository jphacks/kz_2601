import SwiftUI
import MetamonDSL

/// `markdown`: インライン Markdown(**太字** / *斜体* / `code` / [link](url))。解釈に失敗したら生テキスト。
/// リンクは `ExternalURLPolicy` が許す URL だけ生かす(それ以外は文字だけ残す)。生きたリンクは `openURL` が開く
/// = iOS は Safari / 対応アプリ、Mac は既定のブラウザ(#941)。Mac では本文を選択してコピーできる。
struct MarkdownLeaf: View {
    let c: Component

    var body: some View {
        Text(Self.attributed(c.text ?? ""))
            .tint(RenderTheme.Colors.brandText)
            .frame(maxWidth: .infinity, alignment: .leading)
            .macTextSelection()
    }

    /// 解釈した Markdown。許可されないスキームのリンク(javascript: や相対パス)はリンクを外す。
    static func attributed(_ raw: String) -> AttributedString {
        let options = AttributedString.MarkdownParsingOptions(interpretedSyntax: .inlineOnlyPreservingWhitespace)
        guard var out = try? AttributedString(markdown: raw, options: options) else { return AttributedString(raw) }
        let blocked = out.runs.compactMap { run -> Range<AttributedString.Index>? in
            guard let link = run.link else { return nil }
            return ExternalURLPolicy.openable(link.absoluteString) == nil ? run.range : nil
        }
        for range in blocked { out[range].link = nil }
        return out
    }
}
