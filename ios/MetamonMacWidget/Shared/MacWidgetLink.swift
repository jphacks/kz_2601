import Foundation

/// Mac のウィジェットを押した時の URL(#984)。`tailorit://widget?app=<appId>`。
/// ウィジェット(`widgetURL`)と本体(`MacWidgetLinkReceiver`)の両方にコンパイルする。
/// iOS のウィジェットの `metamon://widget` とは別(Mac の URL スキームは `tailorit`)。
enum MacWidgetLink {
    static let scheme = "tailorit"
    static let host = "widget"

    /// appId が無い(アプリ未作成・古いスナップショット)時は本体を前に出すだけの URL
    static func url(appId: String?) -> URL {
        var c = URLComponents()
        c.scheme = scheme
        c.host = host
        if let appId, !appId.isEmpty { c.queryItems = [URLQueryItem(name: "app", value: appId)] }
        return c.url ?? URL(string: "\(scheme)://\(host)")!
    }

    static func isWidget(_ url: URL) -> Bool {
        url.scheme?.lowercased() == scheme && url.host?.lowercased() == host
    }

    /// 開くアプリ。ウィジェットの URL でない・appId が無い時は nil
    static func appId(from url: URL) -> String? {
        guard isWidget(url) else { return nil }
        let id = URLComponents(url: url, resolvingAgainstBaseURL: false)?.queryItems?.first { $0.name == "app" }?.value
        return id?.isEmpty == false ? id : nil
    }
}
