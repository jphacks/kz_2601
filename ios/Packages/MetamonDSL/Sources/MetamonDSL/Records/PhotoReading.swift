import Foundation

/// `photoField.analyze` の 1 項目(#418): 写真から何を読むか(kind)と、結果を入れる field(into)。
public struct PhotoAnalyze: Decodable, Sendable, Hashable {
    public enum Kind: String, Decodable, Sendable {
        case text, amount, date, classify, describe, jointAngle
        case unknown

        public init(from decoder: Decoder) throws { self = Lenient.decode(Self.self, from: decoder, fallback: .unknown) }
    }

    public var kind: Kind
    public var into: String
    /// classify の候補(nil = into の enum の options)
    public var options: [String]?
    /// jointAngle の関節(knee / leftKnee など)
    public var joint: String?

    public init(kind: Kind, into: String, options: [String]? = nil, joint: String? = nil) {
        self.kind = kind
        self.into = into
        self.options = options
        self.joint = joint
    }

    /// 文字認識(OCR)が要るか。
    public var needsText: Bool { [.text, .amount, .date, .classify, .describe].contains(kind) }
    /// 画像分類のラベルが要るか。
    public var needsLabels: Bool { kind == .classify || kind == .describe }
}

/// 読み取った 1 つの値。into の型に合わせて `jsonValue(for:)` で保存形式にする。
public enum PhotoReadingValue: Equatable, Sendable {
    case text(String)
    case number(Double)
    case date(DateComponents)

    /// into の型に入らない組み合わせは nil(backend の検証で弾かれるが、古い DSL でも落とさない)。
    public func jsonValue(for type: FieldType, timeZone: TimeZone = .current) -> JSONValue? {
        switch (self, type) {
        case (.text(let s), .string), (.text(let s), .text), (.text(let s), .enum):
            let t = s.trimmingCharacters(in: .whitespacesAndNewlines)
            return t.isEmpty ? nil : .string(t)
        case (.number(let n), .integer), (.number(let n), .currency):
            return .number(n.rounded())
        case (.number(let n), .number):
            return .number((n * 10).rounded() / 10)
        case (.date(let c), .date):
            guard let y = c.year, let m = c.month, let d = c.day else { return nil }
            return .string(String(format: "%04d-%02d-%02d", y, m, d))
        case (.date(var c), .datetime):
            var cal = Calendar(identifier: .gregorian)
            cal.timeZone = timeZone
            c.calendar = cal
            c.timeZone = timeZone
            guard let date = cal.date(from: c) else { return nil }
            let f = ISO8601DateFormatter()
            f.timeZone = timeZone
            return .string(f.string(from: date))
        default:
            return nil
        }
    }
}

// MARK: - レシート(金額・日付)

/// OCR の行からレシートの合計金額と日付を拾う(純関数・テスト対象)。
public enum ReceiptParser {
    /// 合計を表す語。小計・点数は含めない。
    static let totalWords = ["総合計", "合計", "お会計", "お買上計", "お買上げ計", "ご請求額", "お支払金額", "お支払い金額", "税込", "total"]
    static let excludedWords = ["小計", "点数", "お預", "預り", "お釣", "釣銭", "おつり"]

    /// 全角数字・記号を半角へ。OCR が円記号を `\` と読むことがあるので ¥ に寄せる。
    public static func normalize(_ s: String) -> String {
        var out = s.applyingTransform(.fullwidthToHalfwidth, reverse: false) ?? s
        out = out.replacingOccurrences(of: "￥", with: "¥").replacingOccurrences(of: "\\", with: "¥")
        return out
    }

    private static let numberRe = try! NSRegularExpression(pattern: #"(¥)?\s*(\d{1,3}(?:[,.]\d{3})+|\d+)\s*(円)?"#)

    /// 行の中の金額候補(値・¥/円が付いているか)。8 桁以上(電話番号・会員番号)は外す。
    static func amounts(in line: String) -> [(value: Double, marked: Bool)] {
        let ns = line as NSString
        return numberRe.matches(in: line, range: NSRange(location: 0, length: ns.length)).compactMap { m in
            let digits = ns.substring(with: m.range(at: 2)).replacingOccurrences(of: ",", with: "").replacingOccurrences(of: ".", with: "")
            guard digits.count <= 7, let v = Double(digits) else { return nil }
            // 「3点」「2個」は数量
            let after = m.range.location + m.range.length
            if after < ns.length, ["点", "個", "%", "％"].contains(ns.substring(with: NSRange(location: after, length: 1))) { return nil }
            return (v, m.range(at: 1).location != NSNotFound || m.range(at: 3).location != NSNotFound)
        }
    }

    /// 合計の行(無ければ次の行)の金額。合計の行が無ければ ¥/円 付きの最大額。
    public static func amount(lines raw: [String]) -> Double? {
        let lines = raw.map(normalize)
        var totals: [Double] = []
        for (i, line) in lines.enumerated() {
            let lower = line.lowercased()
            guard totalWords.contains(where: { lower.contains($0) }), !excludedWords.contains(where: { line.contains($0) }) else { continue }
            var found = amounts(in: line)
            // 「合計」と金額が別の行に分かれて読まれることがある
            if found.isEmpty, i + 1 < lines.count { found = amounts(in: lines[i + 1]) }
            if let best = (found.filter(\.marked).isEmpty ? found : found.filter(\.marked)).map(\.value).max(), best > 0 {
                totals.append(best)
            }
        }
        if let t = totals.max() { return t }
        return lines.filter { line in !excludedWords.contains(where: { line.contains($0) }) }
            .flatMap(amounts(in:)).filter(\.marked).map(\.value).filter { $0 > 0 }.max()
    }

    private static let dateRe = try! NSRegularExpression(pattern: #"((?:19|20)\d{2})\s*[年/\-.]\s*(\d{1,2})\s*[月/\-.]\s*(\d{1,2})"#)
    private static let reiwaRe = try! NSRegularExpression(pattern: #"令和\s*(\d{1,2}|元)\s*年\s*(\d{1,2})\s*月\s*(\d{1,2})\s*日"#)
    private static let timeRe = try! NSRegularExpression(pattern: #"(?<!\d)([01]?\d|2[0-3])\s*[:時]\s*([0-5]\d)(?!\d)"#)

    /// 最初に見つかった日付(西暦・令和)。同じ行に時刻があれば時・分も入れる。
    public static func date(lines raw: [String]) -> DateComponents? {
        for line in raw.map(normalize) {
            let ns = line as NSString
            let range = NSRange(location: 0, length: ns.length)
            var c: DateComponents?
            if let m = dateRe.firstMatch(in: line, range: range) {
                c = DateComponents(year: Int(ns.substring(with: m.range(at: 1))), month: Int(ns.substring(with: m.range(at: 2))), day: Int(ns.substring(with: m.range(at: 3))))
            } else if let m = reiwaRe.firstMatch(in: line, range: range) {
                let y = ns.substring(with: m.range(at: 1))
                c = DateComponents(year: 2018 + (y == "元" ? 1 : Int(y) ?? 0), month: Int(ns.substring(with: m.range(at: 2))), day: Int(ns.substring(with: m.range(at: 3))))
            }
            guard var found = c, isValid(found) else { continue }
            if let t = timeRe.firstMatch(in: line, range: range) {
                found.hour = Int(ns.substring(with: t.range(at: 1)))
                found.minute = Int(ns.substring(with: t.range(at: 2)))
            }
            return found
        }
        return nil
    }

    static func isValid(_ c: DateComponents) -> Bool {
        guard let y = c.year, let m = c.month, let d = c.day, (1900...2100).contains(y), (1...12).contains(m), (1...31).contains(d) else { return false }
        let cal = Calendar(identifier: .gregorian)
        guard let date = cal.date(from: c) else { return false }
        return cal.component(.day, from: date) == d // 2/30 などを弾く
    }
}

// MARK: - 分類(Apple Intelligence が無い端末のフォールバック)

/// 画像分類のラベル(英語)と OCR の文字から候補を 1 つ選ぶ。Foundation Models が使えない時だけ使う。
public enum PhotoClassifier {
    /// 候補名に含まれる日本語 → それを示す Vision のラベル(部分一致)。
    static let synonyms: [(ja: String, labels: [String])] = [
        ("食", ["food", "meal", "dish", "fruit", "vegetable", "bread", "dessert", "cake", "sushi", "noodle", "pizza", "salad", "sandwich", "rice", "meat"]),
        ("ごはん", ["food", "meal", "dish", "rice"]),
        ("料理", ["food", "meal", "dish"]),
        ("飲", ["drink", "beverage", "coffee", "tea", "beer", "wine", "juice", "bottle"]),
        ("カフェ", ["coffee", "cafe", "latte"]),
        ("犬", ["dog", "canine", "puppy"]),
        ("猫", ["cat", "feline", "kitten"]),
        ("鳥", ["bird"]),
        ("魚", ["fish"]),
        ("花", ["flower", "blossom", "rose", "tulip", "petal"]),
        ("観葉", ["houseplant", "foliage", "potted", "plant"]),
        ("多肉", ["succulent", "cactus", "aloe"]),
        ("植物", ["plant", "foliage", "leaf", "tree"]),
        ("木", ["tree"]),
        ("本", ["book", "magazine", "publication"]),
        ("書類", ["document", "paper", "text", "receipt"]),
        ("レシート", ["receipt", "document", "paper"]),
        ("服", ["clothing", "shirt", "dress", "jacket", "coat", "apparel", "jeans"]),
        ("靴", ["shoe", "footwear", "sneaker", "boot"]),
        ("交通", ["car", "vehicle", "train", "bus", "bicycle", "taxi", "railway", "airplane"]),
        ("車", ["car", "vehicle", "automobile"]),
        ("電車", ["train", "railway", "subway"]),
        ("日用品", ["toiletry", "detergent", "household", "tissue", "soap", "toothbrush"]),
        ("薬", ["medicine", "pill", "drug", "capsule"]),
        ("景色", ["landscape", "scenery", "sky", "mountain", "sea", "beach", "outdoor"]),
        ("風景", ["landscape", "scenery", "sky", "mountain", "sea", "beach", "outdoor"]),
        ("海", ["sea", "ocean", "beach", "water"]),
        ("山", ["mountain", "hill"]),
        ("空", ["sky", "cloud"]),
        ("人", ["people", "person", "adult", "child", "portrait", "face"]),
        ("家電", ["appliance", "electronics", "computer", "television", "phone"]),
    ]

    /// 最も当てはまる候補(どれにも当てはまらなければ nil)。labels は信頼度順。
    public static func classify(options: [String], labels: [String], text: String) -> String? {
        let labels = labels.map { $0.lowercased() }
        let text = ReceiptParser.normalize(text).lowercased()
        var best: (option: String, score: Double)?
        for option in options {
            let o = option.lowercased()
            var score = 0.0
            if !o.isEmpty, text.contains(o) { score += 2 }
            var keys = [o]
            keys += synonyms.filter { o.contains($0.ja) }.flatMap(\.labels)
            for (rank, label) in labels.enumerated() where keys.contains(where: { label.contains($0) }) {
                score += 1.0 / Double(rank + 1)
            }
            if score > (best?.score ?? 0) { best = (option, score) }
        }
        return best?.option
    }

    /// ラベルの日本語(説明のフォールバック用)。知らないラベルは英語のまま。
    public static func japanese(_ label: String) -> String {
        let l = label.lowercased()
        let words: [String: String] = [
            "food": "食べ物", "meal": "食事", "dish": "料理", "fruit": "果物", "vegetable": "野菜", "drink": "飲み物", "coffee": "コーヒー",
            "dog": "犬", "cat": "猫", "bird": "鳥", "fish": "魚", "flower": "花", "plant": "植物", "tree": "木",
            "book": "本", "document": "書類", "paper": "紙", "text": "文字", "receipt": "レシート",
            "people": "人", "person": "人", "adult": "大人", "child": "子ども", "car": "車", "train": "電車", "bicycle": "自転車",
            "sky": "空", "mountain": "山", "sea": "海", "beach": "浜辺", "building": "建物", "outdoor": "屋外", "indoor": "屋内",
            "shoe": "靴", "clothing": "服", "computer": "パソコン", "phone": "電話", "cake": "ケーキ", "bread": "パン",
        ]
        return words[l] ?? label.replacingOccurrences(of: "_", with: " ")
    }
}

// MARK: - 骨格(関節の角度)

/// Vision の人体の関節(必要なものだけ)。座標は画像の正規化座標(0〜1)。
public enum PoseKeypoint: String, CaseIterable, Sendable {
    case nose, neck, root
    case leftShoulder, rightShoulder, leftElbow, rightElbow, leftWrist, rightWrist
    case leftHip, rightHip, leftKnee, rightKnee, leftAnkle, rightAnkle

    /// 骨格の線(重ねて描く)。
    public static let bones: [(PoseKeypoint, PoseKeypoint)] = [
        (.nose, .neck), (.neck, .leftShoulder), (.neck, .rightShoulder),
        (.leftShoulder, .leftElbow), (.leftElbow, .leftWrist), (.rightShoulder, .rightElbow), (.rightElbow, .rightWrist),
        (.neck, .root), (.root, .leftHip), (.root, .rightHip),
        (.leftHip, .leftKnee), (.leftKnee, .leftAnkle), (.rightHip, .rightKnee), (.rightKnee, .rightAnkle),
    ]
}

public struct PosePoint: Equatable, Sendable {
    public var x: Double
    public var y: Double
    public var confidence: Double

    public init(x: Double, y: Double, confidence: Double = 1) {
        self.x = x
        self.y = y
        self.confidence = confidence
    }
}

public typealias BodyPose = [PoseKeypoint: PosePoint]

/// 関節の角度(度)。`analyze.joint` の名前 → (端, 頂点, 端)。
public enum PoseAngle {
    /// 信頼度がこれ未満の点は使わない。
    public static let minConfidence = 0.3

    static let triplets: [String: (PoseKeypoint, PoseKeypoint, PoseKeypoint)] = [
        "leftKnee": (.leftHip, .leftKnee, .leftAnkle), "rightKnee": (.rightHip, .rightKnee, .rightAnkle),
        "leftElbow": (.leftShoulder, .leftElbow, .leftWrist), "rightElbow": (.rightShoulder, .rightElbow, .rightWrist),
        "leftHip": (.leftShoulder, .leftHip, .leftKnee), "rightHip": (.rightShoulder, .rightHip, .rightKnee),
        "leftShoulder": (.leftElbow, .leftShoulder, .leftHip), "rightShoulder": (.rightElbow, .rightShoulder, .rightHip),
    ]

    /// 頂点 b での a-b-c の角度(0〜180 度)。長さ 0 の辺は nil。
    public static func degrees(_ a: PosePoint, vertex b: PosePoint, _ c: PosePoint) -> Double? {
        let (v1x, v1y, v2x, v2y) = (a.x - b.x, a.y - b.y, c.x - b.x, c.y - b.y)
        let n = (v1x * v1x + v1y * v1y).squareRoot() * (v2x * v2x + v2y * v2y).squareRoot()
        guard n > 1e-9 else { return nil }
        let cos = Swift.max(-1, Swift.min(1, (v1x * v2x + v1y * v2y) / n))
        return acos(cos) * 180 / .pi
    }

    /// `joint` の角度。正規化座標は画像の縦横比(aspect = 幅 / 高さ)でピクセルの比に戻してから測る。
    /// knee / elbow / hip / shoulder は左右のうち信頼度の低い点が高い方。
    public static func angle(joint: String, in pose: BodyPose, aspect: Double = 1) -> Double? {
        let sides = triplets[joint] != nil ? [joint] : ["left", "right"].map { $0 + joint.prefix(1).uppercased() + joint.dropFirst() }
        let candidates: [(angle: Double, confidence: Double)] = sides.compactMap { name in
            guard let (a, b, c) = triplets[name], let pa = pose[a], let pb = pose[b], let pc = pose[c] else { return nil }
            let conf = Swift.min(pa.confidence, pb.confidence, pc.confidence)
            guard conf >= minConfidence else { return nil }
            let scale = { (p: PosePoint) in PosePoint(x: p.x * aspect, y: p.y, confidence: p.confidence) }
            return degrees(scale(pa), vertex: scale(pb), scale(pc)).map { ($0, conf) }
        }
        return candidates.max { $0.confidence < $1.confidence }?.angle
    }
}
