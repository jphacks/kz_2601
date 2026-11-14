// 審査用ビルドは共有リンクを外す(iOS の ShareAppSheet と同じ)
#if !APP_STORE_REVIEW
import AppKit
import MetamonDSL

/// 共有シートの「LINE / Slack / Discord で送る」(#974)。Mac はアプリがあればアプリ、無ければ Web を開く。
/// - LINE: 文面を渡せる共有画面(`https://line.me/R/share?text=`・未インストールなら Web 版)
/// - Slack / Discord: 文面を渡す公式の口が無いので、文面をコピーしてからアプリ(無ければ Web)を開く
enum MacShareTarget: String, CaseIterable, Identifiable {
    case line, slack, discord

    var id: String { rawValue }

    var label: String {
        switch self {
        case .line: L10n.tr("LINE で送る")
        case .slack: L10n.tr("Slack に送る")
        case .discord: L10n.tr("Discord に送る")
        }
    }

    var icon: String { ShareTarget(rawValue).defaultIcon }

    /// 開く前に文面をクリップボードに入れる(Slack / Discord は貼り付けてもらう)
    var copiesText: Bool { self != .line }

    /// 送る文面。iOS の共有シートと同じ「Tailor It で「名前」を使ってみて」+ リンク
    static func message(appName: String, link: URL) -> String {
        L10n.tr("Tailor It で「\(appName)」を使ってみて") + "\n" + link.absoluteString
    }

    /// 送り先に合わせた文面(Slack / Discord は Markdown 記号を落とす・iOS の shareButton と同じ)
    func text(appName: String, link: URL) -> String {
        ShareTarget(rawValue).format(Self.message(appName: appName, link: link))
    }

    /// 開く URL の候補(先頭から、開けるものを 1 つ)。最後の https はいつでも開ける
    func urls(appName: String, link: URL) -> [URL] {
        switch self {
        case .line:
            return [ShareTarget.lineShareURL(text: text(appName: appName, link: link))].compactMap { $0 }
        case .slack:
            return ["slack://open", "https://slack.com/"].compactMap(URL.init(string:))
        case .discord:
            return ["discord://", "https://discord.com/app"].compactMap(URL.init(string:))
        }
    }

    /// 開けるか: https は常に(ブラウザ)、それ以外はそのスキームを開けるアプリがある時
    static func canOpen(_ url: URL, workspace: NSWorkspace = .shared) -> Bool {
        url.scheme?.lowercased() == "https" || workspace.urlForApplication(toOpen: url) != nil
    }

    /// 送る: (要るなら)文面をコピーし、開ける最初の URL を開く。開いた URL を返す
    @MainActor
    @discardableResult
    func send(appName: String, link: URL, board: NSPasteboard = .general,
              canOpen: (URL) -> Bool = { MacShareTarget.canOpen($0) },
              open: (URL) -> Void = { NSWorkspace.shared.open($0) }) -> URL? {
        if copiesText {
            board.clearContents()
            board.setString(text(appName: appName, link: link), forType: .string)
        }
        guard let url = urls(appName: appName, link: link).first(where: canOpen) else { return nil }
        open(url)
        return url
    }

    /// 送った後の案内(Slack / Discord は貼り付けてもらう)
    var sentNotice: String? {
        switch self {
        case .line: nil
        case .slack: L10n.tr("文面をコピーしました。Slack に貼り付けて送ってください")
        case .discord: L10n.tr("文面をコピーしました。Discord に貼り付けて送ってください")
        }
    }
}
#endif
