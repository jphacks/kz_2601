import Foundation

/// JSON over HTTP の最小クライアント。Bearer トークンは `TokenProvider` から都度取得。
enum HTTP {
    static func send(_ method: String, baseURL: URL, path: String, body: (any Encodable)?, token: any TokenProvider) async throws -> Data {
        guard let url = URL(string: path, relativeTo: baseURL) else { throw APIError.invalidURL }
        var req = URLRequest(url: url)
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
            throw APIError.http(status: status, body: String(data: data, encoding: .utf8) ?? "")
        }
        return data
    }
}
