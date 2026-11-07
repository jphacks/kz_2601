// 審査用ビルド(APP_STORE_REVIEW・#452 G4)ではバイナリから外す(aiButton のオンライン版を持たない)。
// 外れると AIClient の既定の text(unsupported)になる。
#if !APP_STORE_REVIEW
import Foundation
import MetamonDSL

/// `POST {base}/v1/text`(#417・契約: backend/internal/ai/text.go / openapi-ai.yaml `/text`)。
extension HTTPAIClient {
    public func text(_ request: AITextRequest) async throws -> AITextResult {
        let data: Data
        do {
            data = try await HTTP.send("POST", baseURL: baseURL, path: "v1/text", body: request, token: token,
                                   timeout: request.isLong ? 75 : 30) // long は出力 1500 トークン(サーバーは 60 秒まで待つ)
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
        // #316: サーバーも Pro を確かめる(端末の判定が古い時など)。共有のアプリの参加者は作成者の Pro が要る(課金画面は出さない)
        case ProRequired.code: ProRequired(e)?.creatorNeedsPro == true ? AITextError.creatorNeedsPro : AITextError.needsPro
        case APIError.aiPausedCode: AITextError.failed("AI を一時的に止めています。しばらくしてからもう一度お試しください")
        case "payload_too_large": AITextError.failed("材料の記録が多すぎます。記録を減らしてからお試しください")
        default: e
        }
    }
}
#endif
