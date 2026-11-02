import Foundation

/// `flashcards`(#264)の山札の状態と計算。描画(FlashcardsLeaf)から切り離してテストする。
/// どこまで進んだかは部品の表示状態だけに持ち、保存しない。保存するのは toggle の field(覚えた / まだ)だけ。
public struct FlashcardDeck: Equatable, Sendable {
    /// この回で答えたカードの id(答えた順)。
    public private(set) var answered: [String] = []
    /// 今のカードの裏を見ているか。
    public private(set) var flipped = false
    /// 保存待ちの「覚えた / まだ」(楽観的 UI。保存の結果が返ったら `settle` で外す)。
    public private(set) var pending: [String: Bool] = [:]

    public init() {}

    // MARK: - 山札

    /// toggle の field を boolean で比べる filter か(eq / neq と true / false)。そうなら出すカードが覚えたもの(true)か、まだのもの(false)か。
    public static func learnedFilter(_ f: Filter, toggle: String?) -> Bool? {
        guard let toggle, !toggle.isEmpty, f.field == toggle, f.valueExpr == nil else { return nil }
        let b: Bool
        switch f.value {
        case .bool(let v): b = v
        case .string("true"): b = true
        case .string("false"): b = false
        default: return nil
        }
        switch f.op {
        case "eq": return b
        case "neq": return !b
        default: return nil
        }
    }

    /// 覚えたか(保存待ちを優先)。保存値の無いカードは「まだ」。bool 以外("true" / 1)も寛容に読む。
    public static func isLearned(_ r: Record, toggle: String, pending: [String: Bool] = [:]) -> Bool {
        if let p = pending[r.id] { return p }
        switch r[toggle] {
        case .bool(let b): return b
        case .number(let n): return n != 0
        case .string(let s): return s == "true"
        default: return false
        }
    }

    /// 出題するカード。`learned` が非 nil なら覚えた / まだ で絞る(値の無いカード = まだ)。並びは登録順(createdAt の昇順・同じなら id)。
    public static func cards(_ records: [Record], toggle: String?, learned: Bool?, pending: [String: Bool] = [:]) -> [Record] {
        var out = records
        if let learned, let toggle {
            out = out.filter { isLearned($0, toggle: toggle, pending: pending) == learned }
        }
        return out.sorted { a, b in
            let x = a.createdAt ?? "", y = b.createdAt ?? ""
            return x == y ? a.id < b.id : x < y
        }
    }

    // MARK: - 今のカード・進み具合

    /// まだ答えていない最初のカード。全部答えたら nil(完了)。
    public func current(in cards: [Record]) -> Record? {
        cards.first { !answered.contains($0.id) }
    }

    /// 「n / 全体」。全体 = 答えたカード + 残り(覚えて山札から外れたカードも答えた数に入る)。
    public func progress(in cards: [Record]) -> (position: Int, total: Int) {
        let remaining = cards.filter { !answered.contains($0.id) }.count
        let total = answered.count + remaining
        return (min(answered.count + 1, total), total)
    }

    /// 1 枚以上あって全部答えた(完了)か。最初から 0 枚の時は false(空の表示)。
    public func isDone(in cards: [Record]) -> Bool {
        !answered.isEmpty && current(in: cards) == nil
    }

    // MARK: - 操作

    public mutating func flip() { flipped.toggle() }

    /// 今のカードに答えて次へ。toggle があれば保存する差分(`[toggle: bool]`)を返し、保存待ちに積む。無ければ nil(めくるだけ)。
    @discardableResult
    public mutating func answer(_ card: Record, learned: Bool, toggle: String?) -> [String: JSONValue]? {
        if !answered.contains(card.id) { answered.append(card.id) }
        flipped = false
        guard let toggle, !toggle.isEmpty else { return nil }
        pending[card.id] = learned
        return [toggle: .bool(learned)]
    }

    /// 保存の結果が返った時。その後に別の答えをしていたら(もう一度の回など)その表示は残す。
    public mutating func settle(_ id: String, requested: Bool) {
        if pending[id] == requested { pending[id] = nil }
    }

    /// もう一度(答えた記録を消して最初から)。保存待ちはそのまま。
    public mutating func restart() {
        answered = []
        flipped = false
    }
}
