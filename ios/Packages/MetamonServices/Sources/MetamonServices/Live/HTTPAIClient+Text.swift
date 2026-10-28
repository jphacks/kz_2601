import Foundation
import MetamonDSL

/// `POST {base}/v1/text`(#417・契約: backend/internal/ai/text.go / openapi-ai.yaml `/text`)。
extension HTTPAIClient {
    public func text(_ request: AITextRequest) async throws -> AITextResult {
        let data: Data
        do {
            data = try await HTTP.send("POST", baseURL: baseURL, path: "v1/text", body: request, token: token, timeout: 30)
        } catch let e as APIError {
            throw Self.textError(e)
        }
        guard let result = AITextResult.parseServer(data, for: request) else { throw AITextError.badOutput }
        return result
    }

    /// サーバーの英語の理由を画面の文言に寄せる。利用上限(rate_limited)はサーバーの日本語をそのまま出す。
    static func textError(_ e: APIError) -> Error {
        switch e.code ?? "" {
        case "refused": AITextError.refused
        case "bad_output": AITextError.badOutput
        case "rate_limited": e
        case "payload_too_large": AITextError.failed("材料の記録が多すぎます。記録を減らしてからお試しください")
        default: e
        }
    }
}
