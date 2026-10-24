import Foundation

/// JSON over HTTP の最小クライアント。Bearer トークンは `TokenProvider` から都度取得。
/// エラー本文は backend 共通の `{error:{code,message}}` を解釈して `APIError.server` にする(人間向け message を toast に出せる)。
enum HTTP {
    /// 既定タイムアウト。ai の生成(Claude 呼び出し)を待つので長め。
    static let defaultTimeout: TimeInterval = 45

    static func send(
        _ method: String,
        baseURL: URL,
        path: String,
        body: (any Encodable)?,
        token: any TokenProvider,
        timeout: TimeInterval = defaultTimeout
    ) async throws -> Data {
        guard let url = URL(string: path, relativeTo: baseURL) else { throw APIError.invalidURL }
        var req = URLRequest(url: url, timeoutInterval: timeout)
        req.httpMethod = method
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        if let t = try await token.idToken() {
            req.setValue("Bearer \(t)", forHTTPHeaderField: "Authorization")
        }
        if let body {
            req.httpBody = try JSONEncoder().encode(body)
        }
        let (data, resp) = try await URLSession.shared.data(for: req)
        let status = (resp as? HTTPURLResponse)?.statusCode ?? -1
        guard (200..<300).contains(status) else {
            if let env = try? JSONDecoder().decode(ErrorEnvelope.self, from: data) {
                throw APIError.server(status: status, code: env.error.code, message: env.error.message)
            }
            throw APIError.http(status: status, body: String(data: data, encoding: .utf8) ?? "")
        }
        return data
    }

    struct ErrorEnvelope: Decodable {
        struct Inner: Decodable {
            var code: String
            var message: String
        }
        var error: Inner
    }
}
