import Foundation
import MetamonDSL

/// `POST /v1/text`(#417)のモック。毎回同じ文を返す(UI テスト・予備端末用。本番ビルドの live では使わない)。
extension MockAIClient {
    static let mockIdeas = ["変顔で自撮りを 1 枚", "好きな歌をワンフレーズ", "語尾を「にゃん」にして 1 分話す", "得意なモノマネを 1 つ",
                            "最近あった小さな失敗を話す", "全員を 1 人ずつほめる", "10 秒で早口言葉", "利き手と逆の手で名前を書く",
                            "今日のラッキーカラーを発表", "次の乾杯の音頭をとる"]

    public func text(_ request: AITextRequest) async throws -> AITextResult {
        try await Task.sleep(for: delay / 2)
        let result: AITextResult?
        switch request.shape {
        case .line:
            result = AITextResult.normalize(text: "今日もよくがんばりました。少し休んでいきましょう(モック)", items: nil, fields: nil, for: request)
        case .list:
            result = AITextResult.normalize(text: nil, items: Array(Self.mockIdeas.prefix(request.resolvedCount)), fields: nil, for: request)
        case .fields:
            let fields = (request.items ?? []).map { AITextField(name: $0, value: "\($0)の案(モック)") }
            result = AITextResult.normalize(text: nil, items: nil, fields: fields, for: request)
        }
        guard let result else { throw AITextError.badOutput }
        return result
    }
}
