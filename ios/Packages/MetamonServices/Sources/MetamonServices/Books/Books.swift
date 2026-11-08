import Foundation
import MetamonDSL

/// ISBN から本の情報を引く(DSL の lookupBook・`GET /v1/lookup/book`)。端末は appconfig にだけ聞き、外部へ直接つながらない。
public protocol BookLookupClient: Sendable {
    func lookup(isbn: String) async throws -> BookInfo
}

/// live の時だけ差し込む(モック・インメモリでは nil = 「手で入力してください」)。
public enum BookLookupServiceKey: ServiceKey {
    public static var defaultValue: (any BookLookupClient)? { nil }
}

extension Services {
    public var books: (any BookLookupClient)? {
        get { self[BookLookupServiceKey.self] }
        set { self[BookLookupServiceKey.self] = newValue }
    }
}

/// appconfig の `v1/lookup/book`。
public struct HTTPBookLookupClient: BookLookupClient {
    public let baseURL: URL
    public let token: any TokenProvider

    public init(baseURL: URL, token: any TokenProvider) {
        self.baseURL = baseURL
        self.token = token
    }

    public func lookup(isbn: String) async throws -> BookInfo {
        var c = URLComponents()
        c.path = "v1/lookup/book"
        c.queryItems = [URLQueryItem(name: "isbn", value: isbn)]
        let data = try await HTTP.send("GET", baseURL: baseURL, path: c.string ?? "v1/lookup/book", body: nil, token: token, timeout: 20)
        return try JSONDecoder().decode(BookInfo.self, from: data)
    }
}

extension APIError {
    /// 本の情報の取得を止めている 503(`details.reason = disabled`)。既定で止めている(openBD の利用条件・2026-11-08)。
    /// つながらない 503 とは分けて「今使えません」と出す。
    public var isBookLookupOff: Bool {
        guard case .server(503, _, _, let d) = self, case .string("disabled")? = d["reason"] else { return false }
        return true
    }
}
