import Foundation

/// `openUrlButton.app`: 名前で開くアプリ・サービス。公開されている形のリンクだけを作る(backend `dsl.OpenApps`・`open_app.go` と同じ決まり)。
/// `to`(リンク・ユーザー名・住所・番号)が形に合わなければ、そのサービスのトップを開く(phone だけは番号が無いと開かない)。
public enum OpenApp: String, Sendable, CaseIterable {
    case paypay, line, slack, discord, x, instagram, maps, mail, phone

    /// 未知・省略は nil(backend は enum で弾く)。
    public init?(_ raw: String?) {
        guard let raw, let app = OpenApp(rawValue: raw) else { return nil }
        self = app
    }

    /// ボタンの既定ラベル(`label` 省略時)。
    public var defaultLabel: String {
        switch self {
        case .paypay: L("PayPay を開く")
        case .line: L("LINE を開く")
        case .slack: L("Slack を開く")
        case .discord: L("Discord を開く")
        case .x: L("X を開く")
        case .instagram: L("Instagram を開く")
        case .maps: L("地図で開く")
        case .mail: L("メールを書く")
        case .phone: L("電話をかける")
        }
    }

    /// 既定の SF Symbols(`icon` 省略時)。
    public var defaultIcon: String {
        switch self {
        case .paypay: "yensign.circle"
        case .line: "bubble.left.fill"
        case .slack: "number"
        case .discord: "gamecontroller"
        case .x: "at"
        case .instagram: "camera"
        case .maps: "map"
        case .mail: "envelope"
        case .phone: "phone"
        }
    }

    /// `to` に書けるリンクのホスト(サブドメインも可)。
    var linkHosts: [String] {
        switch self {
        case .paypay: ["paypay.ne.jp"]
        case .line: ["line.me", "lin.ee"]
        case .slack: ["slack.com"]
        case .discord: ["discord.com", "discord.gg"]
        case .x: ["x.com", "twitter.com"]
        case .instagram: ["instagram.com"]
        case .maps, .mail, .phone: []
        }
    }

    /// そのサービスのトップ(`to` が無い・形に合わない時)。
    var home: [URL] {
        let s: [String] = switch self {
        case .paypay: ["https://paypay.ne.jp/"]
        case .line: ["https://line.me/R/nv/chat"] // LINE URL scheme: チャットのタブ
        case .slack: ["slack://open", "https://slack.com/"] // Slack deep link。アプリが無ければ Web
        case .discord: ["https://discord.com/app"]
        case .x: ["https://x.com/"]
        case .instagram: ["https://www.instagram.com/"]
        case .maps: ["https://maps.apple.com/"]
        case .mail: ["mailto:"]
        case .phone: []
        }
        return s.compactMap(URL.init(string:))
    }

    /// 開く候補。先頭から開き、開けなければ次を試す(slack:// が無い端末は Web)。空 = 開ける物が無い。
    public func urls(to raw: String?) -> [URL] {
        let to = (raw ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
        guard !to.isEmpty else { return home }
        if let link = link(to) { return [link] }
        switch self {
        case .x:
            guard let user = OpenApp.username(to, max: 15, extra: "_") else { return home }
            return [URL(string: "https://x.com/\(user)")].compactMap { $0 }
        case .instagram:
            guard let user = OpenApp.username(to, max: 30, extra: "._") else { return home }
            return [URL(string: "https://www.instagram.com/\(user)/")].compactMap { $0 }
        case .maps:
            guard let q = ShareTarget.percentEncoded(to) else { return home }
            return [URL(string: "https://maps.apple.com/?q=\(q)")].compactMap { $0 }
        case .mail:
            return [ShareTarget.mailURL(to: to, body: nil)].compactMap { $0 }
        case .phone:
            guard let n = OpenApp.phoneNumber(to) else { return [] }
            return [URL(string: "tel:\(n)")].compactMap { $0 }
        case .paypay, .line, .slack, .discord:
            return home
        }
    }

    /// `to` がこのサービスの https リンクならその URL(user@host や別ホストは nil)。
    func link(_ s: String) -> URL? {
        guard !linkHosts.isEmpty, let url = URL(string: s), url.scheme == "https", url.user == nil,
              let host = url.host?.lowercased() else { return nil }
        return linkHosts.contains { host == $0 || host.hasSuffix("." + $0) } ? url : nil
    }

    /// `@name` / `name`(英数字と extra の記号・max 文字まで)→ name。
    static func username(_ s: String, max: Int, extra: String) -> String? {
        let name = s.hasPrefix("@") ? String(s.dropFirst()) : s
        let ok = CharacterSet(charactersIn: "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789" + extra)
        guard (1...max).contains(name.count), name.unicodeScalars.allSatisfy({ ok.contains($0) }) else { return nil }
        return name
    }

    /// 電話番号 → tel: に入れる形(先頭の + と数字だけ)。全角は半角にする。数字が 2〜20 桁でない・他の文字が混ざれば nil。
    static func phoneNumber(_ s: String) -> String? {
        let n = s.precomposedStringWithCompatibilityMapping
        let allowed = CharacterSet(charactersIn: "+0123456789-() ")
        guard n.unicodeScalars.allSatisfy({ allowed.contains($0) }) else { return nil }
        let digits = n.filter { ("0"..."9").contains($0) }
        guard (2...20).contains(digits.count) else { return nil }
        return (n.hasPrefix("+") ? "+" : "") + digits
    }

    static func isEmail(_ s: String) -> Bool {
        s.range(of: #"^[^@\s/?#&]+@[^@\s/?#&]+\.[^@\s/?#&]+$"#, options: .regularExpression) != nil
    }
}
