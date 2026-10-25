import Foundation
import MetamonDSL

/// JSON over HTTP の最小クライアント。Bearer トークンは `TokenProvider` から都度取得。
/// エラー本文は backend 共通の `{error:{code,message,details?}}` を解釈して `APIError.server` にする(人間向け message を UI に出せる)。
///
/// 復帰の自動化(#60・ai / appconfig 共通):
/// - **401**: `TokenProvider.idToken(forceRefresh: true)` で取り直し、値が変わっていれば同じリクエストを **1 回だけ** 再送。それでも 401 なら `APIError`(status 401)を投げ、呼び出し側がサインイン画面へ戻す
/// - **429**: `Retry-After`(秒 or HTTP-date・上限 `maxRetryAfter`)だけ待って **1 回だけ** 再送。ヘッダが無ければ `defaultRetryAfter`
/// - それ以外(5xx / タイムアウト / 圏外)は自動再送しない(UI の「もう一度」に委ねる。POST の二重実行を避ける)
enum HTTP {
    /// 既定タイムアウト。ai の生成(Claude 呼び出し)を待つので長め。
    static let defaultTimeout: TimeInterval = 45
    /// 429 の自動再送で待つ上限 / 既定(秒)。これを超える `Retry-After` は待たずに失敗にして UI に出す。
    static let maxRetryAfter: TimeInterval = 15
    static let defaultRetryAfter: TimeInterval = 2

    /// URLCache を使わないセッション。appconfig の `GET /dsl` は `Cache-Control: no-cache` + ETag だが、
    /// URLSession.shared は再検証せずに古い 200(旧 version)を返すことがあり、起動時復元が 1 つ前の version になった。
    /// 鮮度は自前の `If-None-Match` と `DocumentCache` で管理するので HTTP キャッシュは切る。
    /// テストは `URLProtocol` を登録した configuration で差し替える(`makeSession`)。
    nonisolated(unsafe) static var session: URLSession = makeSession()

    /// 429 待ちの sleep。テストでは記録だけして即戻す。
    nonisolated(unsafe) static var sleep: @Sendable (TimeInterval) async -> Void = { seconds in
        try? await Task.sleep(for: .seconds(seconds))
    }

    static func makeSession(protocolClasses: [AnyClass]? = nil) -> URLSession {
        let config = URLSessionConfiguration.ephemeral
        config.urlCache = nil
        config.requestCachePolicy = .reloadIgnoringLocalAndRemoteCacheData
        config.waitsForConnectivity = false
        if let protocolClasses { config.protocolClasses = protocolClasses }
        return URLSession(configuration: config)
    }

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

    /// ステータスを問わず返す(304 などを呼び出し側で扱う)。転送エラーだけ throw。401 / 429 の 1 回再送はここで行う。
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
        if let body {
            req.httpBody = try JSONEncoder().encode(body)
        }
        let bearer = try await token.idToken()
        var r = try await perform(req, bearer: bearer)

        if r.status == 401, let bearer {
            // トークン失効(Firebase ID トークンは 1 時間)。取り直して値が変わった時だけ同じリクエストを 1 回再送
            let fresh = try await token.idToken(forceRefresh: true)
            if let fresh, fresh != bearer {
                r = try await perform(req, bearer: fresh)
            }
        }
        if r.status == 429 {
            let wait = retryAfter(r) ?? defaultRetryAfter
            if wait <= maxRetryAfter {
                await sleep(wait)
                r = try await perform(req, bearer: try await token.idToken())
            }
        }
        return r
    }

    private static func perform(_ request: URLRequest, bearer: String?) async throws -> Response {
        var req = request
        if let bearer { req.setValue("Bearer \(bearer)", forHTTPHeaderField: "Authorization") }
        let (data, resp) = try await session.data(for: req)
        let http = resp as? HTTPURLResponse
        return Response(status: http?.statusCode ?? -1, data: data, headers: http?.allHeaderFields ?? [:])
    }

    /// `Retry-After` を秒に。`120`(秒)と HTTP-date の両方を受ける。無ければ nil。
    static func retryAfter(_ r: Response) -> TimeInterval? {
        guard let raw = r.header("Retry-After")?.trimmingCharacters(in: .whitespaces), !raw.isEmpty else { return nil }
        if let seconds = TimeInterval(raw) { return max(0, seconds) }
        let f = DateFormatter()
        f.locale = Locale(identifier: "en_US_POSIX")
        f.timeZone = TimeZone(secondsFromGMT: 0)
        f.dateFormat = "EEE, dd MMM yyyy HH:mm:ss zzz"
        if let date = f.date(from: raw) { return max(0, date.timeIntervalSinceNow) }
        return nil
    }

    static func check(_ r: Response) throws -> Data {
        guard (200..<300).contains(r.status) else { throw error(from: r) }
        return r.data
    }

    static func error(from r: Response) -> APIError {
        var details: [String: JSONValue] = [:]
        if let wait = retryAfter(r) { details[APIError.retryAfterKey] = .number(wait) }
        if let env = try? JSONDecoder().decode(ErrorEnvelope.self, from: r.data) {
            details.merge(env.error.details ?? [:]) { _, server in server }
            return .server(status: r.status, code: env.error.code, message: env.error.message, details: details)
        }
        if !details.isEmpty {
            return .server(status: r.status, code: "http_\(r.status)", message: String(data: r.data, encoding: .utf8) ?? "", details: details)
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
