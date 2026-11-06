import Foundation
import MetamonDSL

/// JSON over HTTP の最小クライアント。Bearer トークンは `TokenProvider` から都度取得。
/// エラー本文は backend 共通の `{error:{code,message,details?}}` を解釈して `APIError.server` にする(人間向け message を UI に出せる)。
///
/// 復帰の自動化(#60・ai / appconfig 共通):
/// - **401**: `TokenProvider.idToken(forceRefresh: true)` で取り直し、値が変わっていれば同じリクエストを **1 回だけ** 再送。それでも 401 なら `APIError`(status 401)を投げ、呼び出し側がサインイン画面へ戻す
/// - **401 `app_check_failed`**(#318・backend `APP_CHECK_MODE=enforce`): App Check のトークンを取り直して **1 回だけ** 再送。
///   ID トークンの失効ではないので取り直さない。それでも弾かれたら `FailureKind.appCheckFailed`(サインイン画面へは戻さない)
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

    /// App Check のトークン(#318)。Firebase があれば `FirebaseBootstrap` が入れる。nil ならヘッダを付けない(モック・plist 無し・テスト)。
    nonisolated(unsafe) static var appCheck: (any AppCheckTokenProvider)?

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
        let req = try request(method, baseURL: baseURL, path: path, body: body, timeout: timeout, headers: headers)
        let bearer = try await token.idToken()
        var check = await appCheck?.token(forceRefresh: false)
        var r = try await perform(req, bearer: bearer, appCheck: check)

        if r.status == 401, isAppCheckRejection(r), let provider = appCheck {
            // App Check のトークンが弾かれた(期限切れ・取得失敗)。取り直して値が変わった時だけ 1 回再送
            let fresh = await provider.token(forceRefresh: true)
            if let fresh, fresh != check {
                check = fresh
                r = try await perform(req, bearer: bearer, appCheck: check)
            }
        }
        if r.status == 401, !isAppCheckRejection(r), let bearer {
            // トークン失効(Firebase ID トークンは 1 時間)。取り直して値が変わった時だけ同じリクエストを 1 回再送
            let fresh = try await token.idToken(forceRefresh: true)
            if let fresh, fresh != bearer {
                r = try await perform(req, bearer: fresh, appCheck: check)
            }
        }
        if r.status == 429 {
            let wait = retryAfter(r) ?? defaultRetryAfter
            if wait <= maxRetryAfter {
                await sleep(wait)
                r = try await perform(req, bearer: try await token.idToken(), appCheck: check)
            }
        }
        return r
    }

    /// 認証ヘッダ以外を揃えたリクエスト(`exchange` と `openStream` で共用)。
    static func request(
        _ method: String,
        baseURL: URL,
        path: String,
        body: (any Encodable)?,
        timeout: TimeInterval,
        headers: [String: String]
    ) throws -> URLRequest {
        guard let url = URL(string: path, relativeTo: baseURL) else { throw APIError.invalidURL }
        var req = URLRequest(url: url, cachePolicy: .reloadIgnoringLocalAndRemoteCacheData, timeoutInterval: timeout)
        req.httpMethod = method
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        ClientInfo.current.apply(to: &req) // #302
        for (k, v) in headers { req.setValue(v, forHTTPHeaderField: k) }
        if let body {
            req.httpBody = try JSONEncoder().encode(body)
        }
        return req
    }

    /// 本文を少しずつ読むリクエスト(SSE・#636)。ステータスを問わず返し、401 / 429 の再送はしない(呼び出し側が `exchange` に戻す)。
    /// `timeout` は次のバイトが届くまでの待ち(全体の上限ではない)。
    static func openStream(
        _ method: String,
        baseURL: URL,
        path: String,
        body: (any Encodable)?,
        token: any TokenProvider,
        timeout: TimeInterval = defaultTimeout,
        headers: [String: String] = [:]
    ) async throws -> (bytes: URLSession.AsyncBytes, response: HTTPURLResponse?) {
        var req = try request(method, baseURL: baseURL, path: path, body: body, timeout: timeout, headers: headers)
        if let bearer = try await token.idToken() { req.setValue("Bearer \(bearer)", forHTTPHeaderField: "Authorization") }
        if let check = await appCheck?.token(forceRefresh: false) { req.setValue(check, forHTTPHeaderField: AppCheckHeader.name) }
        let (bytes, resp) = try await session.bytes(for: req)
        return (bytes, resp as? HTTPURLResponse)
    }

    /// 401 が App Check で弾かれたものか(本文の `error.code`)。
    static func isAppCheckRejection(_ r: Response) -> Bool {
        (try? JSONDecoder().decode(ErrorEnvelope.self, from: r.data))?.error.code == AppCheckHeader.failedCode
    }

    private static func perform(_ request: URLRequest, bearer: String?, appCheck: String?) async throws -> Response {
        var req = request
        if let bearer { req.setValue("Bearer \(bearer)", forHTTPHeaderField: "Authorization") }
        if let appCheck { req.setValue(appCheck, forHTTPHeaderField: AppCheckHeader.name) }
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
