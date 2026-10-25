import SwiftUI

/// プライバシーポリシー・利用規約(#246・5.1.1(i))。公開 URL は未定なので、決まるまではアプリに同梱した
/// `docs/legal/*.md` を表示する。URL が決まったら下の `publicURL` だけ差し替える(App Store Connect にも同じ URL を登録)。
enum LegalDocument: String, Identifiable, CaseIterable {
    case privacyPolicy = "privacy-policy"
    case terms

    var id: String { rawValue }

    /// 公開 URL(未定 = nil)。ここが唯一の設定箇所。
    var publicURL: URL? {
        switch self {
        case .privacyPolicy: nil
        case .terms: nil
        }
    }

    var title: LocalizedStringKey {
        switch self {
        case .privacyPolicy: "プライバシーポリシー"
        case .terms: "利用規約"
        }
    }

    var systemImage: String {
        switch self {
        case .privacyPolicy: "hand.raised"
        case .terms: "doc.text"
        }
    }

    /// 同梱の Markdown(project.yml で docs/legal をバンドル)。
    var bundledText: String? {
        guard let url = Bundle.main.url(forResource: rawValue, withExtension: "md") else { return nil }
        return try? String(contentsOf: url, encoding: .utf8)
    }
}

/// 設定・同意画面から開く行。URL があればブラウザ、無ければ同梱文書。
struct LegalDocumentLink: View {
    let document: LegalDocument

    var body: some View {
        if let url = document.publicURL {
            Link(destination: url) { Label(document.title, systemImage: document.systemImage) }
                .accessibilityIdentifier("legal.\(document.rawValue)")
        } else {
            NavigationLink {
                LegalDocumentView(document: document)
            } label: {
                Label(document.title, systemImage: document.systemImage)
            }
            .accessibilityIdentifier("legal.\(document.rawValue)")
        }
    }
}

/// 同梱 Markdown の簡易表示(見出し・箇条書き・段落。行内の **太字** / リンクは AttributedString に任せる)。
struct LegalDocumentView: View {
    let document: LegalDocument

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: Theme.Spacing.sm) {
                if let text = document.bundledText {
                    ForEach(Array(MarkdownBlock.parse(text).enumerated()), id: \.offset) { _, block in
                        block.view
                    }
                } else {
                    Text("文書を読み込めませんでした").foregroundStyle(.secondary)
                }
            }
            .padding(Theme.Spacing.lg)
            .frame(maxWidth: .infinity, alignment: .leading)
            .textSelection(.enabled)
        }
        .navigationTitle(document.title)
        .navigationBarTitleDisplayMode(.inline)
        .accessibilityIdentifier("legal.view.\(document.rawValue)")
    }
}

enum MarkdownBlock {
    case heading(level: Int, text: String)
    case bullet(String)
    case paragraph(String)
    case quote(String)
    case rule

    static func parse(_ text: String) -> [MarkdownBlock] {
        var blocks: [MarkdownBlock] = []
        var paragraph: [String] = []
        func flush() {
            if !paragraph.isEmpty { blocks.append(.paragraph(paragraph.joined(separator: " "))) }
            paragraph = []
        }
        for raw in text.components(separatedBy: .newlines) {
            let line = raw.trimmingCharacters(in: .whitespaces)
            if line.isEmpty || line.hasPrefix("<!--") { flush(); continue }
            if line == "---" { flush(); blocks.append(.rule); continue }
            if line.hasPrefix("#") {
                flush()
                let level = line.prefix { $0 == "#" }.count
                blocks.append(.heading(level: level, text: line.dropFirst(level).trimmingCharacters(in: .whitespaces)))
            } else if line.hasPrefix("- ") || line.hasPrefix("* ") {
                flush()
                blocks.append(.bullet(String(line.dropFirst(2))))
            } else if line.hasPrefix(">") {
                flush()
                blocks.append(.quote(line.dropFirst().trimmingCharacters(in: .whitespaces)))
            } else if line.hasPrefix("|") {
                flush()
                blocks.append(.paragraph(line)) // 表はそのまま 1 行で出す
            } else {
                paragraph.append(line)
            }
        }
        flush()
        return blocks
    }

    private static func inline(_ s: String) -> Text {
        let attributed = (try? AttributedString(markdown: s, options: .init(interpretedSyntax: .inlineOnlyPreservingWhitespace))) ?? AttributedString(s)
        return Text(attributed)
    }

    @ViewBuilder var view: some View {
        switch self {
        case .heading(let level, let text):
            Self.inline(text)
                .font(level <= 1 ? .title2.bold() : level == 2 ? .headline : .subheadline.bold())
                .padding(.top, level <= 2 ? Theme.Spacing.md : Theme.Spacing.sm)
        case .bullet(let text):
            HStack(alignment: .firstTextBaseline, spacing: Theme.Spacing.sm) {
                Text(verbatim: "•")
                Self.inline(text)
            }
            .font(.callout)
        case .paragraph(let text):
            Self.inline(text).font(.callout)
        case .quote(let text):
            Self.inline(text)
                .font(.callout)
                .padding(Theme.Spacing.sm)
                .frame(maxWidth: .infinity, alignment: .leading)
                .background(Color.orange.opacity(0.12), in: RoundedRectangle(cornerRadius: Theme.Radius.md, style: .continuous))
        case .rule:
            Divider()
        }
    }
}
