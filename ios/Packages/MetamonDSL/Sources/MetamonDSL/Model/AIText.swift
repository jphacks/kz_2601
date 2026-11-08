import Foundation

/// aiButton `task: "generate"`(#417)の出力の形。
public enum AITextShape: String, Codable, Sendable, CaseIterable {
    case line, list, fields
}

/// 上限(DSL の検証・`POST /v1/text` と同じ値)。
public enum AITextLimits {
    public static let maxInstruction = 500
    public static let maxItems = 10
    public static let maxInputBytes = 4096
    public static let maxRecords = 20
    public static let defaultCount = 3
    public static let maxItemCharacters = 200
    /// 要約・書き直しなど(task が generate 以外)のオンライン版の答えの上限(backend `TextMaxTaskRunes`)。
    public static let maxTaskCharacters = 1200
    /// length long(backend `TextMaxLongInputBytes` / `TextMaxLongRunes`)。
    public static let maxLongInputBytes = 16384
    public static let maxLongCharacters = 3000

    /// 写真つき(aiButton inputPhoto・backend `TextMaxImageBase64` / `TextMaxImageSide`)。
    public static let maxImageBase64 = 400 * 1024
    public static let maxImageSide = 1024

    /// 依頼の長さに合わせた材料の上限。
    public static func maxInputBytes(long: Bool) -> Int { long ? maxLongInputBytes : maxInputBytes }

    /// UTF-8 で `maxBytes` 以下に切る(文字の途中では切らない)。
    public static func clip(_ s: String, maxBytes: Int = maxInputBytes) -> String {
        guard s.utf8.count > maxBytes else { return s }
        var out = ""
        var used = 0
        for ch in s {
            let n = String(ch).utf8.count
            if used + n > maxBytes { break }
            out.append(ch)
            used += n
        }
        return out
    }
}

/// 1 回分の依頼。`POST /v1/text` の body(openapi `TextRequest`)と同じ形で、端末内 AI にも使う。
public struct AITextRequest: Codable, Equatable, Sendable {
    public var instruction: String
    public var shape: AITextShape
    /// list の個数(list 以外は nil)
    public var count: Int?
    /// fields の項目名(fields 以外は nil)
    public var items: [String]?
    /// 材料(最近の記録・フォームの値)。4 KB まで
    public var input: String?
    /// ja | en
    public var locale: String?
    /// summarize | classify | rewrite | extract | suggest のオンライン版(nil = generate)。shape は line 固定・input 必須
    public var task: String?
    /// classify の候補(classify 以外は nil)
    public var options: [String]?
    /// "long" = 長い文(オンライン AI だけ・材料 16 KB)。nil = short
    public var length: String?
    /// ボタンがあるアプリ(オンライン版だけ・ホストが付ける)。共有のアプリの参加者はサーバーが作成者の Pro で判定する
    public var appId: String?
    /// aiButton inputPhoto の写真 1 枚(オンライン AI だけ)。nil = 写真なし
    public var image: AITextImage?
    /// aiChat の会話(task "chat"・古い順・最後は利用者・オンライン AI だけ)。nil = 会話でない
    public var messages: [AITextMessage]?

    public init(instruction: String, shape: AITextShape, count: Int? = nil, items: [String]? = nil,
                input: String? = nil, locale: String? = nil, task: String? = nil, options: [String]? = nil,
                length: String? = nil, appId: String? = nil, image: AITextImage? = nil, messages: [AITextMessage]? = nil) {
        self.instruction = instruction
        self.shape = shape
        self.count = count
        self.items = items
        self.input = input
        self.locale = locale
        self.task = task
        self.options = options
        self.length = length == "long" ? "long" : nil
        self.appId = appId
        self.image = image
        self.messages = messages
    }

    /// 同じ依頼に appId を付ける(空なら付けない)。写真などほかの値はそのまま。
    public func inApp(_ appId: String) -> AITextRequest {
        var r = self
        r.appId = appId.isEmpty ? nil : appId
        return r
    }

    /// 長い文の依頼か(端末内 AI を使わない)。
    public var isLong: Bool { length == "long" }

    /// aiChat の返事の依頼か(端末内 AI を使わない)。答えは要約などと同じく 1 つの文(long は長い文)。
    public var isChat: Bool { task == "chat" }

    /// 入力を変える task(要約など)か。答えは 1 つの文で、output の field に入れる。
    public var isTransform: Bool { task != nil && task != "generate" }

    /// list の実際の個数(既定 3・1〜10)。
    public var resolvedCount: Int { Swift.min(Swift.max(count ?? AITextLimits.defaultCount, 1), AITextLimits.maxItems) }
}

/// `POST /v1/text` の `messages` の 1 発言(role は user | assistant)。
public struct AITextMessage: Codable, Equatable, Sendable {
    public var role: String
    public var text: String

    public init(role: String, text: String) {
        self.role = role
        self.text = text
    }
}

/// `POST /v1/text` の `image`(JPEG・長い辺 1024px・base64 400 KB まで)。
public struct AITextImage: Codable, Equatable, Sendable {
    public var mediaType: String
    /// 標準の base64(data URL の接頭辞なし)
    public var data: String

    public init(jpegBase64: String) {
        mediaType = "image/jpeg"
        data = jpegBase64
    }
}

public struct AITextField: Codable, Equatable, Sendable {
    public var name: String
    public var value: String

    public init(name: String, value: String) {
        self.name = name
        self.value = value
    }
}

/// 形に合わせて整えた結果。`text` は記録の string/text にそのまま入れる形。
public struct AITextResult: Codable, Equatable, Sendable {
    public var shape: AITextShape
    public var text: String
    public var items: [String]?
    public var fields: [AITextField]?

    public init(shape: AITextShape, text: String, items: [String]? = nil, fields: [AITextField]? = nil) {
        self.shape = shape
        self.text = text
        self.items = items
        self.fields = fields
    }

    /// 生の出力を依頼の形に合わせる(backend の `parseTextOutput` と同じ規則)。合わなければ nil。
    /// line = 最初の空でない行 / list = 1 個以上(count を超えた分は切る)/ fields = 依頼の名前が全部そろう(依頼の順)。
    public static func normalize(text: String?, items: [String]?, fields: [AITextField]?, for request: AITextRequest) -> AITextResult? {
        if request.isLong { return normalizeLong(text) }
        if request.isTransform { return normalizeTransform(text, for: request) }
        switch request.shape {
        case .line:
            guard let line = (text ?? "").split(whereSeparator: \.isNewline).map({ clean(String($0)) }).first(where: { !$0.isEmpty })
            else { return nil }
            return AITextResult(shape: .line, text: line)
        case .list:
            let list = Array((items ?? []).map(clean).filter { !$0.isEmpty }.prefix(request.resolvedCount))
            guard !list.isEmpty else { return nil }
            return AITextResult(shape: .list, text: list.map { "・" + $0 }.joined(separator: "\n"), items: list)
        case .fields:
            var got: [String: String] = [:]
            for f in fields ?? [] {
                let name = f.name.trimmingCharacters(in: .whitespaces)
                let value = clean(f.value)
                if !value.isEmpty, got[name] == nil { got[name] = value }
            }
            var out: [AITextField] = []
            for name in request.items ?? [] {
                guard let v = got[name] else { return nil }
                out.append(AITextField(name: name, value: v))
            }
            guard !out.isEmpty else { return nil }
            return AITextResult(shape: .fields, text: out.map { "\($0.name): \($0.value)" }.joined(separator: "\n"), fields: out)
        }
    }

    /// 要約などの答え(backend `parseTaskOutput` と同じ規則): 全体を残して前後を整え、長ければ切る。
    /// classify は候補そのものか、候補をちょうど 1 つ含む答えだけ受け取る。
    static func normalizeTransform(_ text: String?, for request: AITextRequest) -> AITextResult? {
        let t = (text ?? "").trimmingCharacters(in: CharacterSet(charactersIn: "\"「」").union(.whitespacesAndNewlines))
        guard !t.isEmpty else { return nil }
        if request.task == "classify" {
            let options = request.options ?? []
            if options.contains(t) { return AITextResult(shape: .line, text: t) }
            let hits = options.filter { t.contains($0) }
            return hits.count == 1 ? AITextResult(shape: .line, text: hits[0]) : nil
        }
        return AITextResult(shape: .line, text: String(t.prefix(AITextLimits.maxTaskCharacters)))
    }

    /// 長い文(backend `parseLongOutput` と同じ規則): 段落を残して前後を整え、3000 字で切る。
    static func normalizeLong(_ text: String?) -> AITextResult? {
        let t = (text ?? "").trimmingCharacters(in: CharacterSet(charactersIn: "\"「」").union(.whitespacesAndNewlines))
        guard !t.isEmpty else { return nil }
        return AITextResult(shape: .line, text: String(t.prefix(AITextLimits.maxLongCharacters)))
    }

    /// `POST /v1/text` の 200 本文(openapi `TextResponse`)。サーバーが整えた後でも同じ規則で確かめる。
    public static func parseServer(_ data: Data, for request: AITextRequest) -> AITextResult? {
        guard let raw = try? JSONDecoder().decode(AITextResult.self, from: data) else { return nil }
        return normalize(text: raw.text, items: raw.items, fields: raw.fields, for: request)
    }

    /// 端末内 AI の構造化出力(`{"text"}` / `{"items"}` / fields は `{"f0": …}` = 依頼の items の順)。
    public static func parseOnDevice(json: String, for request: AITextRequest) -> AITextResult? {
        guard let obj = (try? JSONSerialization.jsonObject(with: Data(json.utf8))) as? [String: Any] else { return nil }
        let fields = (request.items ?? []).enumerated().compactMap { i, name in
            (obj["f\(i)"] as? String).map { AITextField(name: name, value: $0) }
        }
        return normalize(text: obj["text"] as? String, items: obj["items"] as? [String], fields: fields, for: request)
    }

    /// 前後の空白・行頭の記号や番号・かぎかっこを落とし、長すぎれば切る。
    static func clean(_ s: String) -> String {
        var t = s.trimmingCharacters(in: .whitespacesAndNewlines)
        if let r = t.range(of: #"^(?:[-*・•●]|\d+[.)、.])\s*"#, options: .regularExpression) { t.removeSubrange(r) }
        t = t.trimmingCharacters(in: CharacterSet(charactersIn: "\"「」").union(.whitespacesAndNewlines))
        return String(t.prefix(AITextLimits.maxItemCharacters))
    }
}

/// generate の失敗の理由(画面にそのまま出す)。
public enum AITextError: LocalizedError, Equatable, Sendable {
    /// オンライン版が無い(プレビュー・旧ホスト)
    case unsupported
    /// AI への送信に同意していない(ホストが同意画面を出した)
    case needsConsent
    /// Tailor It Pro が要る(ホストが課金画面を出した)
    case needsPro
    /// 共有のアプリの作成者の Pro が要る(参加者は払っても直らないので課金画面は出さない)
    case creatorNeedsPro
    /// AI が書くのを断った
    case refused
    /// 答えが形に合わない
    case badOutput
    /// その他(サーバーのメッセージ)
    case failed(String)

    public var errorDescription: String? {
        switch self {
        case .unsupported: "この端末では AI を使えません"
        case .needsConsent: "AI へのデータ送信に同意すると使えます"
        #if APP_STORE_REVIEW
        case .needsPro: "この端末では AI を使えません" // 審査用ビルドは App 内課金を持たない(#450 G1)。この版では来ない
        #else
        case .needsPro: "この端末ではオンラインの AI を使います。Tailor It Pro で使えます"
        #endif
        case .creatorNeedsPro: "この機能は今は使えません。アプリを作った人に知らせてください"
        case .refused: "この内容は AI が書けませんでした"
        case .badOutput: "AI の答えを読み取れませんでした。もう一度お試しください"
        case .failed(let message): message
        }
    }
}
