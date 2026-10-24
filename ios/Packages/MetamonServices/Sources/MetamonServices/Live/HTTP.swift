import Foundation
import MetamonDSL

/// JSON over HTTP の最小クライアント。Bearer トークンは `TokenProvider` から都度取得。
/// エラー本文は backend 共通の `{error:{code,message,details?}}` を解釈して `APIError.server` にする(人間向け message を toast に出せる)。
enum HTTP {
    /// 既定タイムアウト。ai の生成(Claude 呼び出し)を待つので長め。
    static let defaultTimeout: TimeInterval = 45

    /// URLCache を使わないセッション。appconfig の `GET /dsl` は `Cache-Control: no-cache` + ETag だが、
    /// URLSession.shared は再検証せずに古い 200(旧 version)を返すことがあり、起動時復元が 1 つ前の version になった。
    /// 鮮度は自前の `If-None-Match` と `DocumentCache` で管理するので HTTP キャッシュは切る。
    static let session: URLSession = {
        let config = URLSessionConfiguration.ephemeral
        config.urlCache = nil
        config.requestCachePolicy = .reloadIgnoringLocalAndRemoteCacheData
        config.waitsForConnectivity = false
        return URLSession(configuration: config)
    }()

    struct Response {
        var status: Int
        var data: Data
        var headers: [AnyHashable: Any]

        func header(_ name: String) -> String? {
            headers.first { ($0.key as? String)?.caseInsensitiveCompare(name) == .orderedSame }?.value as? String
        }
    }

    /// 2xx の本文だけ返す(それ以外は `APIError`)。
    static func send(
        _ method: String,
        baseURL: URL,
        path: String,
        body: (any Encodable)?,
        token: any TokenProvider,
        timeout: TimeInterval = defaultTimeout,
        headers: [String: String] = [:]
    ) async throws -> Data {
        try check(await exchange(method, baseURL: baseURL, path: path, body: body, token: token, timeout: timeout, headers: headers))
    }

    /// ステータスを問わず返す(304 などを呼び出し側で扱う)。転送エラーだけ throw。
    static func exchange(
        _ method: String,
        baseURL: URL,
        path: String,
        body: (any Encodable)?,
        token: any TokenProvider,
        timeout: TimeInterval = defaultTimeout,
        headers: [String: String] = [:]
    ) async throws -> Response {
        guard let url = URL(string: path, relativeTo: baseURL) else { throw APIError.invalidURL }
        var req = URLRequest(url: url, cachePolicy: .reloadIgnoringLocalAndRemoteCacheData, timeoutInterval: timeout)
        req.httpMethod = method
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        for (k, v) in headers { req.setValue(v, forHTTPHeaderField: k) }
        if let t = try await token.idToken() {
            req.setValue("Bearer \(t)", forHTTPHeaderField: "Authorization")
        }
        if let body {
            req.httpBody = try JSONEncoder().encode(body)
        }
        let (data, resp) = try await session.data(for: req)
        let http = resp as? HTTPURLResponse
        return Response(status: http?.statusCode ?? -1, data: data, headers: http?.allHeaderFields ?? [:])
    }

    static func check(_ r: Response) throws -> Data {
        guard (200..<300).contains(r.status) else { throw error(from: r) }
        return r.data
    }

    static func error(from r: Response) -> APIError {
        if let env = try? JSONDecoder().decode(ErrorEnvelope.self, from: r.data) {
            return .server(status: r.status, code: env.error.code, message: env.error.message, details: env.error.details ?? [:])
        }
        return .http(status: r.status, body: String(data: r.data, encoding: .utf8) ?? "")
    }

    struct ErrorEnvelope: Decodable {
        struct Inner: Decodable {
            var code: String
            var message: String
            var details: [String: JSONValue]?
        }
        var error: Inner
    }
}
