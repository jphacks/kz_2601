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
        /// ja | en。nil は送らない(サーバーが発話から判定)
        var locale: String?
        /// 使ってよい部品の allowlist と無効な機能(#245)。通常ビルドは nil = 送らない(全部使える)
        var allowedComponents: [String]? = ClientCapabilities.allowedComponents
        var disabledFeatures: [String]? = ClientCapabilities.disabledFeatures
    }

    struct GenerateResponse: Decodable {
        var appId: String?
        var version: Int
        var mode: String?
        var dsl: AppDSL
        var summary: String?
        /// #211。省略 = 変化あり
        var unchanged: Bool?
        var meta: Meta?

        struct Meta: Decodable {
            /// #253。壊れていても生成は捨てない(`try?`)
            var publicPersonalData: [PublicPersonalField]?

            enum CodingKeys: String, CodingKey { case publicPersonalData }

            init(from decoder: Decoder) throws {
                let c = try decoder.container(keyedBy: CodingKeys.self)
                publicPersonalData = try? c.decodeIfPresent([PublicPersonalField].self, forKey: .publicPersonalData)
            }
        }
    }

    public func morph(transcript: String, appId: String, currentVersion: Int) async throws -> MorphResult {
        try await morph(transcript: transcript, appId: appId, currentVersion: currentVersion, locale: nil)
    }

    public func morph(transcript: String, appId: String, currentVersion: Int, locale: String?) async throws -> MorphResult {
        do {
            return try await generate(transcript: transcript, appId: appId, baseVersion: currentVersion > 0 ? currentVersion : nil, locale: locale)
        } catch APIError.server(409, _, _, _) {
            // サーバー側 version とズレた(ai のインメモリ store 再起動など)。現行基準で 1 回だけやり直す。
            return try await generate(transcript: transcript, appId: appId, baseVersion: nil, locale: locale)
        }
    }

    private func generate(transcript: String, appId: String, baseVersion: Int?, locale: String?) async throws -> MorphResult {
        let body = GenerateBody(appId: appId, baseVersion: baseVersion, transcript: transcript, locale: locale)
        let data = try await HTTP.send("POST", baseURL: baseURL, path: "v1/generate", body: body, token: token, timeout: timeout)
        let result = try Self.parse(data, appId: appId)
        if let cache, let envelope = Self.envelope(from: data, appId: result.document.appId, version: result.document.version) {
            cache.store(appId: result.document.appId, envelope: envelope)
        }
        return result
    }

    // MARK: - 対話(clarify)

    struct ChatBody: Encodable {
        var appId: String
        var messages: [ChatMessage]
        var locale: String?
        /// create | edit(#184)。nil は送らない
        var mode: ChatMode?
        /// iOS が表示できる追加の kind(#184)。backend はここに無い kind を返さない
        var capabilities: [String] = ChatCapability.all
        /// 使ってよい部品の allowlist と無効な機能(#245)。通常ビルドは nil = 送らない
        var allowedComponents: [String]? = ClientCapabilities.allowedComponents
        var disabledFeatures: [String]? = ClientCapabilities.disabledFeatures
    }

    /// `POST {base}/v1/chat`(契約: backend/internal/ai/chat.go の `ChatRequest` / `ChatResult`・#184 会話の取り決め)。
    public func chat(appId: String, messages: [ChatMessage], locale: String?, mode: ChatMode?) async throws -> ChatTurn {
        let body = ChatBody(appId: appId, messages: messages, locale: locale, mode: mode)
        let data = try await HTTP.send("POST", baseURL: baseURL, path: "v1/chat", body: body, token: token, timeout: timeout)
        return try JSONDecoder().decode(ChatTurn.self, from: data)
    }

    /// `DELETE {base}/v1/chat/{appId}` → 204。失敗は握りつぶす(状態はサーバー側で 30 分で消える)。
    public func resetChat(appId: String) async {
        let id = appId.addingPercentEncoding(withAllowedCharacters: .urlPathAllowed) ?? appId
        _ = try? await HTTP.send("DELETE", baseURL: baseURL, path: "v1/chat/\(id)", body: nil, token: token, timeout: 10)
    }

    static func parse(_ data: Data, appId: String) throws -> MorphResult {
        let res = try JSONDecoder().decode(GenerateResponse.self, from: data)
        let doc = AppDocument(appId: res.appId ?? appId, version: res.version, dsl: res.dsl)
        return MorphResult(document: doc, summary: res.summary, mode: res.mode, unchanged: res.unchanged ?? false,
                           publicPersonalData: res.meta?.publicPersonalData ?? [])
    }

    /// generate 応答から appconfig の `DSLEnvelope` 形(`{appId, version, dsl}`)のバイト列を切り出す(キャッシュ用)。
    static func envelope(from data: Data, appId: String, version: Int) -> Data? {
        guard let obj = try? JSONSerialization.jsonObject(with: data) as? [String: Any], let dsl = obj["dsl"] else { return nil }
        return try? JSONSerialization.data(withJSONObject: ["appId": appId, "version": version, "dsl": dsl])
    }
}
