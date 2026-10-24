import Foundation
import MetamonDSL

/// ai サービス: POST /v1/generate → 適用後の DSL 封筒({appId, version, dsl, ...})
public struct HTTPAIClient: AIClient {
    public let baseURL: URL
    public let token: any TokenProvider

    public init(baseURL: URL, token: any TokenProvider) {
        self.baseURL = baseURL
        self.token = token
    }

    private struct GenerateBody: Encodable {
        var appId: String
        var baseVersion: Int
        var transcript: String
    }

    public func morph(transcript: String, appId: String, currentVersion: Int) async throws -> AppDocument {
        let body = GenerateBody(appId: appId, baseVersion: currentVersion, transcript: transcript)
        return try AppDocument.decode(try await HTTP.send("POST", baseURL: baseURL, path: "v1/generate", body: body, token: token))
    }
}
