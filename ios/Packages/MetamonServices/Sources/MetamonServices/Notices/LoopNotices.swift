// 審査用ビルド(APP_STORE_REVIEW)ではフィードバックと一緒に外す(#374・docs/ios/review-build.md)。
#if !APP_STORE_REVIEW
import Foundation

/// 「あなたの声で直りました」(#374)。送ったフィードバックが直った時に、自己改善ループが送り主に作るお知らせ。
/// 契約: docs/backend/openapi-appconfig.yaml `GET /me/notices`
public struct LoopNotice: Identifiable, Equatable, Sendable, Decodable {
    public var id: String
    public var title: String
    public var body: String
    public var createdAt: Date
    /// nil = 未読
    public var readAt: Date?

    public var isUnread: Bool { readAt == nil }

    public init(id: String, title: String, body: String, createdAt: Date, readAt: Date? = nil) {
        self.id = id
        self.title = title
        self.body = body
        self.createdAt = createdAt
        self.readAt = readAt
    }

    enum CodingKeys: String, CodingKey { case id, title, body, createdAt, readAt }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = try c.decode(String.self, forKey: .id)
        title = try c.decodeIfPresent(String.self, forKey: .title) ?? ""
        body = try c.decodeIfPresent(String.self, forKey: .body) ?? ""
        createdAt = try c.decodeIfPresent(String.self, forKey: .createdAt).flatMap(AppSummary.parseDate) ?? .distantPast
        readAt = try c.decodeIfPresent(String.self, forKey: .readAt).flatMap(AppSummary.parseDate)
    }
}

/// `GET /v1/me/notices` の 1 ページ。`unreadCount` はすべてのページの未読の数。
public struct LoopNoticePage: Equatable, Sendable, Decodable {
    public var items: [LoopNotice]
    public var unreadCount: Int
    public var nextPageToken: String?

    public init(items: [LoopNotice], unreadCount: Int, nextPageToken: String? = nil) {
        self.items = items
        self.unreadCount = unreadCount
        self.nextPageToken = nextPageToken
    }
}

/// お知らせと「直ったら知らせてほしい」(push)の同意。
public protocol LoopNoticesClient: Sendable {
    /// 新しい順。`pageToken` は前のページの `nextPageToken`。
    func notices(pageToken: String?) async throws -> LoopNoticePage
    /// 既読にする(冪等)。
    func markRead(id: String) async throws
    /// push の同意(保存が無い = false)。
    func feedbackFixedConsent() async throws -> Bool
    func setFeedbackFixedConsent(_ enabled: Bool) async throws
}

/// live の時だけ(モックでは nil = 設定の行とフィードバックの「直ったら知らせてほしい」を出さない)。
public enum LoopNoticesServiceKey: ServiceKey {
    public static var defaultValue: (any LoopNoticesClient)? { nil }
}

extension Services {
    public var loopNotices: (any LoopNoticesClient)? {
        get { self[LoopNoticesServiceKey.self] }
        set { self[LoopNoticesServiceKey.self] = newValue }
    }
}

/// appconfig の `v1/me/notices`・`v1/me/notify-consent/feedback-fixed`。
public struct HTTPLoopNoticesClient: LoopNoticesClient {
    public let baseURL: URL
    public let token: any TokenProvider

    public init(baseURL: URL, token: any TokenProvider) {
        self.baseURL = baseURL
        self.token = token
    }

    public func notices(pageToken: String?) async throws -> LoopNoticePage {
        var path = "v1/me/notices"
        if let pageToken, let q = pageToken.addingPercentEncoding(withAllowedCharacters: .urlQueryValueAllowed) {
            path += "?pageToken=\(q)"
        }
        let data = try await HTTP.send("GET", baseURL: baseURL, path: path, body: nil, token: token, timeout: 20)
        return try JSONDecoder().decode(LoopNoticePage.self, from: data)
    }

    public func markRead(id: String) async throws {
        _ = try await HTTP.send("POST", baseURL: baseURL, path: "v1/me/notices/\(id)/read", body: nil, token: token, timeout: 20)
    }

    struct Consent: Codable { var enabled: Bool }

    public func feedbackFixedConsent() async throws -> Bool {
        let data = try await HTTP.send("GET", baseURL: baseURL, path: "v1/me/notify-consent/feedback-fixed", body: nil, token: token, timeout: 20)
        return try JSONDecoder().decode(Consent.self, from: data).enabled
    }

    public func setFeedbackFixedConsent(_ enabled: Bool) async throws {
        _ = try await HTTP.send("PUT", baseURL: baseURL, path: "v1/me/notify-consent/feedback-fixed", body: Consent(enabled: enabled), token: token, timeout: 20)
    }
}

private extension CharacterSet {
    /// クエリの値に使える文字(`&` `=` `+` を除く)。
    static let urlQueryValueAllowed: CharacterSet = {
        var s = CharacterSet.urlQueryAllowed
        s.remove(charactersIn: "&=+")
        return s
    }()
}
#endif
