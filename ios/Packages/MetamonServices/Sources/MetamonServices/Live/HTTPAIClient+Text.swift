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
                                   timeout: request.isLong ? 75 : (request.image != nil || request.isChat ? 45 : 30)) // long は出力 1500 トークン(サーバーは 60 秒まで待つ)・写真と会話は送る分だけ長く
        } catch let e as APIError {
            throw Self.textError(e, photo: request.image != nil, chat: request.isChat)
        }
        guard let result = AITextResult.parseServer(data, for: request) else { throw AITextError.badOutput }
        return result
    }

    /// サーバーの英語の理由を画面の文言に寄せる。利用上限(rate_limited)はサーバーの日本語をそのまま出す。
    static func textError(_ e: APIError, photo: Bool = false, chat: Bool = false) -> Error {
        // inputPhoto: サーバーの写真の検証(413 / 400 の "image …")と、写真の受け付けが止まっている 403
        if photo, case .server(_, let code, let message, _) = e {
            if code == "payload_too_large" || (code == "bad_request" && message.hasPrefix("image")) {
                return AITextError.failed(L("写真を AI に送れませんでした。撮り直してからお試しください"))
            }
            if code == "forbidden" { return AITextError.failed(L("写真を AI に見せる機能は、今は使えません")) }
        }
        // aiChat: 会話が長すぎる(端末で 16 KB に切って送るので普段は来ない)
        if chat, e.code == "payload_too_large" { return AITextError.failed(L("会話が長すぎて AI に送れませんでした。短くしてからお試しください")) }
        return switch e.code ?? "" {
        case "refused": AITextError.refused
        case "bad_output": AITextError.badOutput
        case "rate_limited": e
        // #316: サーバーも Pro を確かめる(端末の判定が古い時など)。共有のアプリの参加者は作成者の Pro が要る(課金画面は出さない)
        case ProRequired.code: ProRequired(e)?.creatorNeedsPro == true ? AITextError.creatorNeedsPro : AITextError.needsPro
        case APIError.aiPausedCode: AITextError.failed(L("AI を一時的に止めています。しばらくしてからもう一度お試しください"))
        case "payload_too_large": AITextError.failed(L("材料の記録が多すぎます。記録を減らしてからお試しください"))
        default: e
        }
    }
}
#endif
