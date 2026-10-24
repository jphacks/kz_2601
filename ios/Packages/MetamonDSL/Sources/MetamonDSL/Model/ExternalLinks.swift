import Foundation

/// `shareButton.shareTo` の送り先 preset。Renderer はこれで ShareLink / LINE 共有 URL を使い分ける。
/// Webhook 投稿(Slack/Discord Incoming Webhook)は #37 中版で別途。ここは URL スキームと共有シートだけ。
public enum ShareTarget: String, Sendable, CaseIterable {
    case any, line, slack, discord

    /// 未知・省略は `.any`(backend は enum で弾くが iOS は寛容)。
    public init(_ raw: String?) { self = raw.flatMap(ShareTarget.init(rawValue:)) ?? .any }

    /// ボタンの既定ラベル(`label` 省略時)。
    public var defaultLabel: String {
        switch self {
        case .any: "共有"
        case .line: "LINE で送る"
        case .slack: "Slack に送る"
        case .discord: "Discord に送る"
        }
    }

    /// 既定の SF Symbols(`icon` 省略時)。
    public var defaultIcon: String {
        switch self {
        case .any: "square.and.arrow.up"
        case .line: "bubble.left.fill"
        case .slack: "number"
        case .discord: "gamecontroller"
        }
    }

    /// LINE の共有画面を開く URL(ユニバーサルリンク。LINE 未インストールなら Web 版に遷移)。
    public static func lineShareURL(text: String) -> URL? {
        var cs = CharacterSet.alphanumerics
        cs.insert(charactersIn: "-._~")
        guard let q = text.addingPercentEncoding(withAllowedCharacters: cs) else { return nil }
        return URL(string: "https://line.me/R/share?text=\(q)")
    }

    /// 送り先に合わせた文面。slack/discord はチャンネル投稿向けに Markdown 記号を落としたプレーンテキストにし、
    /// 先頭に 1 行の見出し記号を付ける(どのアプリに貼っても崩れない)。line/any はそのまま。
    public func format(_ text: String) -> String {
        switch self {
        case .any, .line: text
        case .slack, .discord: "💬 " + ShareTarget.plainText(text)
        }
    }

    /// `**強調**` `__強調__` `` `code` `` と行頭の `# ` `> ` `- ` `* `(見出し/引用/箇条書き記号)を外し、3 連以上の改行を 2 つに詰める。
    public static func plainText(_ text: String) -> String {
        var s = text
        for marker in ["**", "__", "`"] { s = s.replacingOccurrences(of: marker, with: "") }
        s = s.components(separatedBy: "\n")
            .map { $0.replacingOccurrences(of: #"^[#>*\-]+\s+"#, with: "", options: .regularExpression) }
            .joined(separator: "\n")
        while s.contains("\n\n\n") { s = s.replacingOccurrences(of: "\n\n\n", with: "\n\n") }
        return s.trimmingCharacters(in: .whitespacesAndNewlines)
    }
}

/// `openUrlButton` / `link` が開いてよい URL の判定。backend `dsl.OpenURLSchemes` と同じ allowlist。
/// `line` `slack` `discord` は Info.plist の `LSApplicationQueriesSchemes` にも載せる。
public enum ExternalURLPolicy {
    public static let allowedSchemes: Set<String> = ["https", "line", "slack", "discord", "tel", "mailto", "maps", "comgooglemaps"]

    public static func isAllowed(_ url: URL) -> Bool {
        guard let scheme = url.scheme?.lowercased() else { return false }
        return allowedSchemes.contains(scheme)
    }

    /// 許可されたスキームなら URL、そうでなければ nil。
    public static func openable(_ string: String?) -> URL? {
        guard let string, let url = URL(string: string), isAllowed(url) else { return nil }
        return url
    }

    /// `webView` は https のみ。
    public static func webViewURL(_ string: String?) -> URL? {
        guard let string, let url = URL(string: string), url.scheme?.lowercased() == "https", url.host != nil else { return nil }
        return url
    }
}
