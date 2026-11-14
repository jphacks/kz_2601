import Foundation
import MetamonDSL

/// ISBN から本の情報を引く(DSL の lookupBook・`POST /v1/lookup/book`)。端末は appconfig にだけ聞き、外部へ直接つながらない。
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
        // POST: 入力された ISBN を URL に載せない
        let data = try await HTTP.send("POST", baseURL: baseURL, path: "v1/lookup/book", body: LookupBody(isbn: isbn), token: token, timeout: 20)
        return try JSONDecoder().decode(BookInfo.self, from: data)
    }

    private struct LookupBody: Encodable { let isbn: String }
}

extension APIError {
    /// 本の情報の取得を止めている 503(`details.reason = disabled`)。既定で止めている(openBD の利用条件・2026-11-08)。
    /// つながらない 503 とは分けて「今使えません」と出す。
    public var isBookLookupOff: Bool {
        guard case .server(503, _, _, let d) = self, case .string("disabled")? = d["reason"] else { return false }
        return true
    }
}
