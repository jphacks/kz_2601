import Foundation
import MetamonDSL

/// ai サービス `POST {base}/v1/generate`(契約: backend/internal/ai/generate.go の `Request` / `Result`)。
/// - body: `{appId, baseVersion?, transcript, mode:"auto"}`、`Authorization: Bearer <Firebase ID token>`
/// - 200: `{appId, version, previousVersion, mode, dsl, patch?, summary, ...}` → `dsl` と `summary` を使う
/// - 4xx/5xx: `{error:{code,message}}` → `APIError.server`
/// 成功した DSL は `DocumentCache` にも入れる(appconfig から取り直さなくてもオフライン復元できるように)。
public struct HTTPAIClient: AIClient {
    public let baseURL: URL
    public let token: any TokenProvider
    public var timeout: TimeInterval
    public var cache: DocumentCache?

    public init(baseURL: URL, token: any TokenProvider, timeout: TimeInterval = 45, cache: DocumentCache? = .shared) {
        self.baseURL = baseURL
        self.token = token
        self.timeout = timeout
        self.cache = cache
    }

    struct GenerateBody: Encodable {
        var appId: String
        var baseVersion: Int?
        var transcript: String
        var mode = "auto"
    }

    struct GenerateResponse: Decodable {
        var appId: String?
        var version: Int
        var mode: String?
        var dsl: AppDSL
        var summary: String?
    }

    public func morph(transcript: String, appId: String, currentVersion: Int) async throws -> MorphResult {
        do {
            return try await generate(transcript: transcript, appId: appId, baseVersion: currentVersion > 0 ? currentVersion : nil)
        } catch APIError.server(409, _, _, _) {
            // サーバー側 version とズレた(ai のインメモリ store 再起動など)。現行基準で 1 回だけやり直す。
            return try await generate(transcript: transcript, appId: appId, baseVersion: nil)
        }
    }

    private func generate(transcript: String, appId: String, baseVersion: Int?) async throws -> MorphResult {
        let body = GenerateBody(appId: appId, baseVersion: baseVersion, transcript: transcript)
        let data = try await HTTP.send("POST", baseURL: baseURL, path: "v1/generate", body: body, token: token, timeout: timeout)
        let result = try Self.parse(data, appId: appId)
        if let cache, let envelope = Self.envelope(from: data, appId: result.document.appId, version: result.document.version) {
            cache.store(appId: result.document.appId, envelope: envelope)
        }
        return result
    }

    static func parse(_ data: Data, appId: String) throws -> MorphResult {
        let res = try JSONDecoder().decode(GenerateResponse.self, from: data)
        let doc = AppDocument(appId: res.appId ?? appId, version: res.version, dsl: res.dsl)
        return MorphResult(document: doc, summary: res.summary, mode: res.mode)
    }

    /// generate 応答から appconfig の `DSLEnvelope` 形(`{appId, version, dsl}`)のバイト列を切り出す(キャッシュ用)。
    static func envelope(from data: Data, appId: String, version: Int) -> Data? {
        guard let obj = try? JSONSerialization.jsonObject(with: data) as? [String: Any], let dsl = obj["dsl"] else { return nil }
        return try? JSONSerialization.data(withJSONObject: ["appId": appId, "version": version, "dsl": dsl])
    }
}
