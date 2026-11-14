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
        if request.image != nil, request.isTransform, request.task != "classify" {
            // inputPhoto: 写真を読んだ体の決まった文
            guard let out = AITextResult.normalize(text: "写真から読み取った内容です(モック)", items: nil, fields: nil, for: request) else { throw AITextError.badOutput }
            return out
        }
        if request.isChat {
            // aiChat: 最後の発言に返す決まった文
            let last = request.messages?.last?.text ?? ""
            guard var out = AITextResult.normalize(text: "なるほど、「\(last.prefix(20))」ですね。もう少し聞かせてください(モック)", items: nil, fields: nil, for: request) else { throw AITextError.badOutput }
            out.suggest = request.saveTo.map { Self.mockSuggest($0, last: last) } // saveTo: 記録の案
            return out
        }
        if request.isLong {
            // length long: 段落のある長い文
            let text = (1...4).map { "\($0) 段落目の長い文章です。決まった文を返しています(モック)。" }.joined(separator: "\n\n")
            guard let out = AITextResult.normalize(text: text, items: nil, fields: nil, for: request) else { throw AITextError.badOutput }
            return out
        }
        if request.isTransform {
            // 要約など: classify は最初の候補、それ以外は決まった文
            let text = request.task == "classify" ? request.options?.first : "\(request.task ?? "")の結果です(モック)"
            guard let out = AITextResult.normalize(text: text, items: nil, fields: nil, for: request) else { throw AITextError.badOutput }
            return out
        }
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

    /// aiChat saveTo の案: 文字は最後の発言・数は 1・日付は today・選択肢は最初のもの。
    static func mockSuggest(_ saveTo: AITextSaveTo, last: String) -> [String: JSONValue] {
        var out: [String: JSONValue] = [:]
        for f in saveTo.fields {
            switch f.type {
            case "number": out[f.id] = .number(1)
            case "date": out[f.id] = .string(saveTo.today)
            case "enum": if let o = f.options?.first { out[f.id] = .string(o) }
            default: out[f.id] = .string(String(last.prefix(40)))
            }
        }
        return out
    }
}
