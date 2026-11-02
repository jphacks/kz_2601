import Foundation

/// アプリ内のお知らせ(#333)。appconfig `GET /v1/client-config` の `notice`(環境変数 `NOTICE_JSON`)。
/// 障害・メンテナンスの 1 行告知。審査用の版でも出す(運用の機能のため)。
public struct ServiceNotice: Decodable, Equatable, Sendable {
    public enum Severity: String, Decodable, Sendable {
        case info, warning

        public init(from decoder: Decoder) throws {
            // 知らない値(後の版で増えた等)は info として出す
            self = Severity(rawValue: (try? decoder.singleValueContainer().decode(String.self)) ?? "") ?? .info
        }
    }

    public struct Message: Decodable, Equatable, Sendable {
        public var ja: String
        public var en: String?

        public init(ja: String, en: String? = nil) {
            self.ja = ja
            self.en = en
        }
    }

    public var id: String
    public var severity: Severity
    public var message: Message
    public var linkUrl: String?
    public var minIOSBuild: Int?
    public var maxIOSBuild: Int?

    public init(id: String, severity: Severity = .info, message: Message, linkUrl: String? = nil, minIOSBuild: Int? = nil, maxIOSBuild: Int? = nil) {
        self.id = id
        self.severity = severity
        self.message = message
        self.linkUrl = linkUrl
        self.minIOSBuild = minIOSBuild
        self.maxIOSBuild = maxIOSBuild
    }

    private enum CodingKeys: String, CodingKey { case id, severity, message, linkUrl, minIOSBuild, maxIOSBuild }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = try c.decode(String.self, forKey: .id)
        severity = (try? c.decode(Severity.self, forKey: .severity)) ?? .info
        message = try c.decode(Message.self, forKey: .message)
        linkUrl = try? c.decodeIfPresent(String.self, forKey: .linkUrl)
        minIOSBuild = try? c.decodeIfPresent(Int.self, forKey: .minIOSBuild)
        maxIOSBuild = try? c.decodeIfPresent(Int.self, forKey: .maxIOSBuild)
    }

    /// 表示する言語の文面。英語で `en` が無ければ日本語。
    public func text(languageCode: String) -> String {
        if languageCode.lowercased().hasPrefix("en"), let en = message.en?.trimmingCharacters(in: .whitespacesAndNewlines), !en.isEmpty { return en }
        return message.ja.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    /// https のリンクだけ使う(サーバーでも弾くが、念のため)。
    public var linkURL: URL? {
        guard let raw = linkUrl, let url = URL(string: raw), url.scheme == "https", url.host?.isEmpty == false else { return nil }
        return url
    }

    /// この build(CFBundleVersion)に出すか。範囲の指定があって build が読めなければ出さない。
    public func applies(toBuild build: String) -> Bool {
        let min = minIOSBuild.flatMap { $0 > 0 ? $0 : nil }
        let max = maxIOSBuild.flatMap { $0 > 0 ? $0 : nil }
        if min == nil && max == nil { return true }
        guard let current = BuildNumber(build) else { return false }
        if let min, current < BuildNumber(min) { return false }
        if let max, BuildNumber(max) < current { return false }
        return true
    }

    /// 出してよい形か(id と日本語の文面がある)。
    var isDisplayable: Bool {
        !id.trimmingCharacters(in: .whitespaces).isEmpty && !message.ja.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
    }
}

/// 閉じたお知らせの id を覚える(#333)。同じ id は二度と出さない。新しい文面は新しい id で出す。
public struct NoticeDismissals {
    public static let defaultsKey = "serviceNotice.dismissedIDs"
    /// 古いものから捨てる(UserDefaults を太らせない)
    static let limit = 20

    private let defaults: UserDefaults

    public init(defaults: UserDefaults = .standard) {
        self.defaults = defaults
    }

    public func isDismissed(_ id: String) -> Bool { ids.contains(id) }

    public func dismiss(_ id: String) {
        var list = ids.filter { $0 != id }
        list.append(id)
        defaults.set(Array(list.suffix(Self.limit)), forKey: Self.defaultsKey)
    }

    var ids: [String] { defaults.stringArray(forKey: Self.defaultsKey) ?? [] }
}
