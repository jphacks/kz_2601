import Foundation

// 外の API(#1493)。契約: docs/backend/openapi-appconfig.yaml の tag `ext`・docs/backend/features/external-apis.md
// 端末は appconfig だけを呼ぶ。API キーと利用者のトークンは runner だけが持ち、端末には来ない。

/// つないだサービス 1 件(`ExtConnection`)。トークンは含まない。
public struct ExtConnection: Codable, Equatable, Sendable {
    public enum Status: Equatable, Sendable {
        case active
        /// provider がトークンを拒んだ(つなぎ直す)
        case error
        case unknown(String)

        init(raw: String) {
            switch raw {
            case "active": self = .active
            case "error": self = .error
            default: self = .unknown(raw)
            }
        }

        var raw: String {
            switch self {
            case .active: "active"
            case .error: "error"
            case .unknown(let s): s
            }
        }
    }

    public var provider: String
    public var status: Status
    public var scopes: [String]?
    public var connectedAt: Date?
    public var lastUsedAt: Date?

    public init(provider: String, status: Status, scopes: [String]? = nil, connectedAt: Date? = nil, lastUsedAt: Date? = nil) {
        self.provider = provider
        self.status = status
        self.scopes = scopes
        self.connectedAt = connectedAt
        self.lastUsedAt = lastUsedAt
    }

    enum CodingKeys: String, CodingKey { case provider, status, scopes, connectedAt, lastUsedAt }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        provider = try c.decode(String.self, forKey: .provider)
        status = Status(raw: try c.decode(String.self, forKey: .status))
        scopes = try c.decodeIfPresent([String].self, forKey: .scopes)
        connectedAt = try c.decodeIfPresent(String.self, forKey: .connectedAt).flatMap(ExtDate.parse)
        lastUsedAt = try c.decodeIfPresent(String.self, forKey: .lastUsedAt).flatMap(ExtDate.parse)
    }

    public func encode(to encoder: Encoder) throws {
        var c = encoder.container(keyedBy: CodingKeys.self)
        try c.encode(provider, forKey: .provider)
        try c.encode(status.raw, forKey: .status)
        try c.encodeIfPresent(scopes, forKey: .scopes)
        try c.encodeIfPresent(connectedAt.map(ExtDate.format), forKey: .connectedAt)
        try c.encodeIfPresent(lastUsedAt.map(ExtDate.format), forKey: .lastUsedAt)
    }
}

/// つなげるサービス 1 件(`ExtProvider`)。`enabled` が false ならこのサーバーでは今つなげない。
public struct ExtProvider: Codable, Equatable, Sendable {
    public var id: String
    public var nameJa: String
    public var nameEn: String
    public var enabled: Bool

    public init(id: String, nameJa: String, nameEn: String, enabled: Bool = true) {
        self.id = id
        self.nameJa = nameJa
        self.nameEn = nameEn
        self.enabled = enabled
    }
}

/// `GET /v1/me/connections` の答え。`providers` が nil = 古いサーバー(フィールドが無い)。
public struct ExtConnectionList: Decodable, Equatable, Sendable {
    public var connections: [ExtConnection]
    public var providers: [ExtProvider]?

    public init(connections: [ExtConnection], providers: [ExtProvider]?) {
        self.connections = connections
        self.providers = providers
    }
}

/// op の決まった形の答えの 1 行(`ExtRow`)。
public struct ExtRow: Codable, Equatable, Sendable {
    public var title: String
    public var subtitle: String?
    public var value: String?
    public var url: String?

    public init(title: String, subtitle: String? = nil, value: String? = nil, url: String? = nil) {
        self.title = title
        self.subtitle = subtitle
        self.value = value
        self.url = url
    }
}

/// op の答え(`ExtResult`)。読む op は rows・1 件は item・書く op は resultId。
public struct ExtResult: Codable, Equatable, Sendable {
    public var rows: [ExtRow]?
    public var item: ExtRow?
    public var resultId: String?

    public init(rows: [ExtRow]? = nil, item: ExtRow? = nil, resultId: String? = nil) {
        self.rows = rows
        self.item = item
        self.resultId = resultId
    }
}

/// `POST /v1/apps/{appId}/ext/{provider}/{op}` の答え。コインは有料の provider だけ(`X-Coins-Charged` / `X-Coins-Balance`)。
public struct ExtCallResponse: Equatable, Sendable {
    public var result: ExtResult
    public var coinsCharged: Int?
    public var coinsBalance: Int?

    public init(result: ExtResult, coinsCharged: Int? = nil, coinsBalance: Int? = nil) {
        self.result = result
        self.coinsCharged = coinsCharged
        self.coinsBalance = coinsBalance
    }
}

public protocol ExternalServicesClient: Sendable {
    /// `GET /v1/me/connections`(切ったものは出ない)。`supportedFeatures` を渡すと `?supportedFeatures=` で
    /// `externalCall.<id>` の入った provider だけに絞る(空の配列 = 何も出さない・nil = 絞らない)
    func connections(supportedFeatures: [String]?) async throws -> ExtConnectionList
    /// `POST /v1/me/connections/{provider}/start` → 同意の画面の URL
    func startConnection(provider: String, scopes: [String]?) async throws -> URL
    /// `DELETE /v1/me/connections/{provider}`(つないでいなくても成功)
    func disconnect(provider: String) async throws
    /// `POST /v1/apps/{appId}/ext/{provider}/{op}`。`requestId` は有料の provider のコインの押さえの鍵
    func call(appId: String, provider: String, op: String, args: [String: String], requestId: String?) async throws -> ExtCallResponse
}

public extension APIError {
    static let extNotConnectedCode = "ext_not_connected"
    static let extUnavailableCode = "ext_unavailable"
    static let extBusyCode = "ext_busy"

    /// 409: OAuth の要る op で本人がつないでいない・切った・provider が拒んだ
    var isExtNotConnected: Bool { code == Self.extNotConnectedCode }
    /// 503: 外の API を止めている(runner が無い・provider が止まっている)
    var isExtUnavailable: Bool { code == Self.extUnavailableCode }
    /// 503: 混んでいる
    var isExtBusy: Bool { code == Self.extBusyCode }

    /// `details.provider`
    var extProvider: String? {
        if case .server(_, _, _, let d) = self, case .string(let p)? = d["provider"] { return p }
        return nil
    }
}

/// ISO 8601(小数秒あり・なしの両方)
enum ExtDate {
    static func parse(_ s: String) -> Date? {
        let f = ISO8601DateFormatter()
        f.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        if let d = f.date(from: s) { return d }
        f.formatOptions = [.withInternetDateTime]
        return f.date(from: s)
    }

    static func format(_ d: Date) -> String {
        ISO8601DateFormatter().string(from: d)
    }
}

/// provider / op はパスに入るので、openapi の形(英数字・lowerCamel)だけ通す。
enum ExtPath {
    static func isName(_ s: String, max: Int) -> Bool {
        guard let first = s.unicodeScalars.first, s.count >= 2, s.count <= max,
              ("a"..."z").contains(first) else { return false }
        return s.unicodeScalars.allSatisfy { ("a"..."z").contains($0) || ("A"..."Z").contains($0) || ("0"..."9").contains($0) }
    }

    static func provider(_ s: String) throws -> String {
        guard isName(s, max: 32) else { throw APIError.invalidURL }
        return s
    }

    static func op(_ s: String) throws -> String {
        guard isName(s, max: 48) else { throw APIError.invalidURL }
        return s
    }

    static func appId(_ s: String) throws -> String {
        guard !s.isEmpty, let e = s.addingPercentEncoding(withAllowedCharacters: .urlPathAllowed.subtracting(CharacterSet(charactersIn: "/?#"))) else {
            throw APIError.invalidURL
        }
        return e
    }
}

/// appconfig の `ext` の口。
public struct HTTPExternalServicesClient: ExternalServicesClient {
    public let baseURL: URL
    public let token: any TokenProvider

    public init(baseURL: URL, token: any TokenProvider) {
        self.baseURL = baseURL
        self.token = token
    }

    struct StartBody: Encodable { var scopes: [String]? }
    struct StartResponse: Decodable { var authUrl: String }
    struct CallBody: Encodable { var args: [String: String] }

    public func connections(supportedFeatures: [String]?) async throws -> ExtConnectionList {
        var path = "v1/me/connections"
        if let supportedFeatures {
            var q = URLComponents()
            q.queryItems = [URLQueryItem(name: "supportedFeatures", value: supportedFeatures.joined(separator: ","))]
            path += "?" + (q.percentEncodedQuery ?? "")
        }
        let data = try await HTTP.send("GET", baseURL: baseURL, path: path, body: nil, token: token)
        return try JSONDecoder().decode(ExtConnectionList.self, from: data)
    }

    public func startConnection(provider: String, scopes: [String]?) async throws -> URL {
        let p = try ExtPath.provider(provider)
        let body = StartBody(scopes: scopes?.isEmpty == false ? scopes : nil)
        let data = try await HTTP.send("POST", baseURL: baseURL, path: "v1/me/connections/\(p)/start", body: body, token: token)
        let raw = try JSONDecoder().decode(StartResponse.self, from: data).authUrl
        guard let url = URL(string: raw), url.scheme == "https" else { throw APIError.invalidURL }
        return url
    }

    public func disconnect(provider: String) async throws {
        let p = try ExtPath.provider(provider)
        _ = try await HTTP.send("DELETE", baseURL: baseURL, path: "v1/me/connections/\(p)", body: nil, token: token)
    }

    public func call(appId: String, provider: String, op: String, args: [String: String], requestId: String?) async throws -> ExtCallResponse {
        let path = "v1/apps/\(try ExtPath.appId(appId))/ext/\(try ExtPath.provider(provider))/\(try ExtPath.op(op))"
        var headers: [String: String] = [:]
        if let requestId { headers["X-Request-Id"] = requestId }
        let r = try await HTTP.exchange("POST", baseURL: baseURL, path: path, body: CallBody(args: args), token: token, headers: headers)
        let data = try HTTP.check(r)
        let result = try JSONDecoder().decode(ExtResult.self, from: data)
        return ExtCallResponse(result: result,
                               coinsCharged: r.header("X-Coins-Charged").flatMap(Int.init),
                               coinsBalance: r.header("X-Coins-Balance").flatMap(Int.init))
    }
}

/// モック(`METAMON_MOCK`・サーバー無しの起動・テスト)。つなぐと同意の画面を飛ばして成功の戻り先を返す扱い。
public actor MockExternalServicesClient: ExternalServicesClient {
    public private(set) var stored: [ExtConnection]
    /// nil = 古いサーバーのまね(`providers` を返さない)
    public private(set) var providers: [ExtProvider]?
    /// 直前の `connections(supportedFeatures:)` に渡された値(テスト用)
    public private(set) var lastSupportedFeatures: [String]??
    /// 次の呼び出しで投げるエラー(テスト用)
    public var nextError: Error?
    public private(set) var disconnected: [String] = []
    public private(set) var started: [String] = []

    public init(connections: [ExtConnection] = [], providers: [ExtProvider]? = MockExternalServicesClient.defaultProviders) {
        self.stored = connections
        self.providers = providers
    }

    public static let defaultProviders = [
        ExtProvider(id: "fakeOauth", nameJa: "試験用のサービス", nameEn: "Test service"),
        ExtProvider(id: "googleCalendar", nameJa: "Google カレンダー", nameEn: "Google Calendar"),
        ExtProvider(id: "todoist", nameJa: "Todoist", nameEn: "Todoist"),
    ]

    public func setNextError(_ error: Error?) { nextError = error }

    private func throwIfNeeded() throws {
        if let e = nextError {
            nextError = nil
            throw e
        }
    }

    public func connections(supportedFeatures: [String]?) async throws -> ExtConnectionList {
        try throwIfNeeded()
        lastSupportedFeatures = .some(supportedFeatures)
        // サーバーと同じく `externalCall.<id>` で絞る
        let shown = providers.map { list in
            guard let supportedFeatures else { return list }
            let keys = Set(supportedFeatures)
            return list.filter { keys.contains("externalCall.\($0.id)") }
        }
        return ExtConnectionList(connections: stored, providers: shown)
    }

    public func startConnection(provider: String, scopes: [String]?) async throws -> URL {
        try throwIfNeeded()
        _ = try ExtPath.provider(provider)
        started.append(provider)
        // モックは同意の画面を出さず、すぐつないだことにする
        stored.removeAll { $0.provider == provider }
        stored.append(ExtConnection(provider: provider, status: .active, scopes: scopes, connectedAt: Date()))
        return Self.mockAuthURL(provider: provider)
    }

    public func disconnect(provider: String) async throws {
        try throwIfNeeded()
        disconnected.append(provider)
        stored.removeAll { $0.provider == provider }
    }

    public func call(appId: String, provider: String, op: String, args: [String: String], requestId: String?) async throws -> ExtCallResponse {
        try throwIfNeeded()
        let title = args.sorted { $0.key < $1.key }.map(\.value).joined(separator: " ")
        return ExtCallResponse(result: ExtResult(rows: [ExtRow(title: "\(provider).\(op)", subtitle: title.isEmpty ? nil : title, value: "1")]))
    }

    /// モックの同意の URL。アプリはこの URL を見たら Web を開かずに戻り先の成功として扱う。
    public static func mockAuthURL(provider: String) -> URL {
        URL(string: "https://mock.invalid/oauth/\(provider)")!
    }

    public static func isMockAuthURL(_ url: URL) -> Bool { url.host == "mock.invalid" }
}

/// 外の API(#1493)。live では `HTTPExternalServicesClient`、モックでは `MockExternalServicesClient`。
public enum ExternalServicesKey: ServiceKey {
    public static let defaultValue: any ExternalServicesClient = MockExternalServicesClient()
}

extension Services {
    public var external: any ExternalServicesClient {
        get { self[ExternalServicesKey.self] }
        set { self[ExternalServicesKey.self] = newValue }
    }
}
