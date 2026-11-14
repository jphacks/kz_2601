import Foundation

/// `bracket`(トーナメント表)の組み合わせと勝ち上がり。描画(BracketLeaf)と描画の木(RenderTree)が同じ計算を使う。
/// 組み合わせは保存しない: 出場者を seedField の昇順(値の無い人は後ろ)→ 登録順に並べ、2 のべき乗の枠に標準のシード配置で入れる。
/// 勝ち上がりは winsField(勝った回戦の数)だけで決まる。規則は docs/dsl/README.md の bracket の補足。
/// 結果が 1 つでも付いたら組み合わせを固定する(`frozen`): 開始後に追加された出場者は枠に入れない。
public struct BracketDraw: Equatable, Sendable {
    /// 描く出場者の上限(6 回戦)。Go の `dsl.MaxBracketEntrants` と同じ。
    public static let maxEntrants = 64
    /// 読む wins の上限(回戦の数より十分大きい)。
    public static let maxWins = 1_000

    public enum Side: String, Equatable, Sendable { case a, b }

    public struct Match: Equatable, Sendable {
        /// 1 始まりの回戦
        public let round: Int
        public let index: Int
        /// 上の枠・下の枠の出場者。1 回戦の nil は不戦勝の空き枠、2 回戦からの nil は前の試合がまだ決まっていない。
        public let a: Record?
        public let b: Record?
        public let winner: Side?
        /// 1 回戦の不戦勝(相手の枠が空き)。
        public let bye: Bool

        public var winnerRecord: Record? {
            switch winner {
            case .a: a
            case .b: b
            case nil: nil
            }
        }

        /// 勝者を選べる試合か(2 人がそろい、不戦勝でない)。
        public var isPlayable: Bool { a != nil && b != nil && !bye }
    }

    /// 回戦ごとの試合(rounds[0] = 1 回戦)。出場者が 2 人未満なら空。
    public let rounds: [[Match]]
    /// 枠に入れた出場者(シード順)。
    public let entrants: [Record]
    /// 開始後に追加されたため枠に入れなかった人数(`frozen`)。
    public let lateEntrants: Int
    /// `startWrites` で固定前の枠を組み直すため。
    private let seedField: String?

    public var champion: Record? { rounds.last?.first?.winnerRecord }

    /// - Parameters:
    ///   - pending: 保存待ちの勝ち数(楽観的 UI)。保存値より優先する。
    ///   - startedField: 開始時刻の datetime field(`frozen`)。
    public init(records: [Record], winsField: String, seedField: String?, startedField: String? = nil, pending: [String: Int] = [:]) {
        let (pool, late) = Self.frozen(records, winsField: winsField, startedField: startedField)
        let entrants = Array(Self.seeded(pool, seedField: seedField).prefix(Self.maxEntrants))
        self.entrants = entrants
        self.lateEntrants = late
        self.seedField = seedField
        guard entrants.count >= 2 else {
            rounds = []
            return
        }
        let wins = { (r: Record) -> Int in Self.wins(r, field: winsField, pending: pending) }
        var size = 2
        while size < entrants.count { size *= 2 }
        let slots: [Record?] = Self.slotSeeds(size).map { $0 <= entrants.count ? entrants[$0 - 1] : nil }

        var out: [[Match]] = []
        var first: [Match] = []
        for i in 0..<(size / 2) {
            let a = slots[2 * i], b = slots[2 * i + 1]
            let bye = (a == nil) != (b == nil)
            let winner: Side? = bye ? (a == nil ? .b : .a) : Self.winner(a, b, round: 1, wins: wins)
            first.append(Match(round: 1, index: i, a: a, b: b, winner: winner, bye: bye))
        }
        out.append(first)
        var round = 1
        while out[round - 1].count > 1 {
            round += 1
            let prev = out[round - 2]
            var cur: [Match] = []
            for i in 0..<(prev.count / 2) {
                let a = prev[2 * i].winnerRecord, b = prev[2 * i + 1].winnerRecord
                cur.append(Match(round: round, index: i, a: a, b: b, winner: Self.winner(a, b, round: round, wins: wins), bye: false))
            }
            out.append(cur)
        }
        rounds = out
    }

    // MARK: - 組み合わせの固定

    /// 開始後に追加された人を外す。保存済みの wins >= 1 の人がいる時だけ固定する(全部取り消すと外れる)。wins >= 1 の人は常に残す。
    /// - startedField あり(値の入った人がいる): 最初の結果と一緒に、枠に入れた全員へ開始時刻を書く(`startWrites`)。
    ///   勝者の最も早い値を開始とし、それと同じ値(±1 秒)の人と createdAt がそれ以前の人を残す(正確)。
    ///   それより後の値(後から足した人のフォームの既定値 now など)は開始の印とみなさない。勝者に値が無ければ時刻の規則。
    /// - startedField なし・値が無い(古いアプリ): wins >= 1 の人の updatedAt の最も早い時刻を開始とし、createdAt がそれ以前の人を残す。
    ///   勝者がまた勝つと updatedAt が進むので、1 回戦の途中に足した人が後で入ることがある。
    /// 時刻が読めない時は全員。保存待ち(pending)は見ない: 保存前の updatedAt は結果の時刻でないため。
    public static func frozen(_ records: [Record], winsField: String, startedField: String? = nil) -> (entrants: [Record], late: Int) {
        let winners = records.filter { wins($0, field: winsField) >= 1 }
        guard !winners.isEmpty else { return (records, 0) }
        let field = startedField.flatMap { $0.isEmpty ? nil : $0 }
        let times: [(id: String, at: Date)] = field.map { f in records.compactMap { r in ChatThread.date(r[f].stringValue).map { (r.id, $0) } } } ?? []
        // 開始時刻は勝者の値の最も早いもの(勝者は必ず枠の中・開始時に書かれる)。後から足した人の値(フォームの既定値 now)は使わない
        let winnerIds = Set(winners.map(\.id))
        let earliest = times.filter { winnerIds.contains($0.id) }.map(\.at).min()
        // 枠の全員に同じ時刻を 1 度に書くので、その値と同じ(±1 秒)人だけが開始時の出場者
        let stamped = earliest.map { e in times.filter { $0.at.timeIntervalSince(e) <= Self.startTolerance } } ?? []
        let started = earliest ?? winners.compactMap { ChatThread.date($0.updatedAt) }.min()
        guard let started else { return (records, 0) }
        let stampedIds = Set(stamped.map(\.id))
        let kept = records.filter { r in
            if wins(r, field: winsField) >= 1 || stampedIds.contains(r.id) { return true }
            guard let created = ChatThread.date(r.createdAt) else { return true }
            return created <= started
        }
        return (kept, records.count - kept.count)
    }

    /// 同じ開始時刻とみなす幅(`frozen`)。
    static let startTolerance: TimeInterval = 1

    /// 勝ち数の書き込み(`tap` の結果)と一緒に送る startedField の値(recordId → 値)。
    /// 結果が 0 → 1 つになる時: 登録済みの全員(シード順に最大 `maxEntrants`)に now、ほかで値の残っている人は null。
    /// 枠は既存の開始時刻(後から足した人のフォームの既定値など)に頼らず組み直す。結果が 1 → 0 になる時(全部取り消し): 値のある人を null。
    public func startWrites(for update: [String: Int], records: [Record], winsField: String, startedField: String?,
                            pending: [String: Int] = [:], now: String) -> [String: JSONValue] {
        guard let f = startedField, !f.isEmpty else { return [:] }
        let before = records.contains { Self.wins($0, field: winsField, pending: pending) >= 1 }
        let after = records.contains { (update[$0.id] ?? Self.wins($0, field: winsField, pending: pending)) >= 1 }
        var out: [String: JSONValue] = [:]
        if before == after { return out }
        for r in records where !r[f].isNull { out[r.id] = .null }
        if after { for r in Self.seeded(records, seedField: seedField).prefix(Self.maxEntrants) { out[r.id] = .string(now) } }
        return out
    }

    // MARK: - 並べ方

    /// seedField の昇順(数値の無い人は後ろ)→ 登録順(createdAt の昇順・同じなら id)。
    public static func seeded(_ records: [Record], seedField: String?) -> [Record] {
        records.sorted { x, y in
            if let f = seedField, !f.isEmpty {
                let sx = x[f].doubleValue, sy = y[f].doubleValue
                if sx != sy {
                    switch (sx, sy) {
                    case let (l?, r?): return l < r
                    case (.some, nil): return true
                    default: return false
                    }
                }
            }
            let cx = x.createdAt ?? "", cy = y.createdAt ?? ""
            return cx == cy ? x.id < y.id : cx < cy
        }
    }

    /// 枠の順のシード番号(1 始まり)。8 枠なら [1, 8, 4, 5, 2, 7, 3, 6]: 上位どうしは決勝まで当たらず、空き枠は上位シードの相手になる。
    public static func slotSeeds(_ size: Int) -> [Int] {
        var seeds = [1]
        while seeds.count < size {
            let m = seeds.count * 2
            seeds = seeds.flatMap { [$0, m + 1 - $0] }
        }
        return seeds
    }

    /// 勝った回戦の数。保存待ちを優先し、数値でない値は 0。
    public static func wins(_ r: Record, field: String, pending: [String: Int] = [:]) -> Int {
        if let p = pending[r.id] { return p }
        switch r[field] {
        // Int(n) は Int に収まらないと落ちるので先に丸める
        case .number(let n): return n.isFinite ? Int(Swift.min(Swift.max(n, 0), Double(maxWins))) : 0
        case .string(let s): return Swift.max(0, Int(s) ?? 0)
        default: return 0
        }
    }

    /// r 回戦の勝者: wins >= r の方。両方なら値の大きい方、同じなら上の枠。
    static func winner(_ a: Record?, _ b: Record?, round: Int, wins: (Record) -> Int) -> Side? {
        guard let a, let b else { return nil }
        let wa = wins(a), wb = wins(b)
        switch (wa >= round, wb >= round) {
        case (true, true): return wb > wa ? .b : .a
        case (true, false): return .a
        case (false, true): return .b
        case (false, false): return nil
        }
    }

    // MARK: - 勝者を選ぶ

    public enum TapResult: Equatable, Sendable {
        /// 保存する勝ち数(recordId → wins)
        case update([String: Int])
        /// 選べない試合(不戦勝・相手が未定)
        case notPlayable
        /// 先の回戦に進んだ人の試合(先の結果を取り消すまで変えられない)
        case locked
    }

    /// 試合の勝者が上がった先の試合に結果が付いているか(決勝は先が無いので false)。
    public func nextDecided(_ match: Match) -> Bool {
        guard match.round < rounds.count else { return false }
        let next = rounds[match.round]
        let i = match.index / 2
        return i < next.count && next[i].winner != nil
    }

    /// 試合の side の人をタップした時(この表の先の試合を見て止める)。
    public func tap(_ match: Match, side: Side, winsField: String, pending: [String: Int] = [:]) -> TapResult {
        Self.tap(match, side: side, winsField: winsField, pending: pending, nextDecided: nextDecided(match))
    }

    /// 試合の side の人をタップした時。勝者でなければ勝者に(相手が勝者なら戻す)、勝者なら取り消す(相手も wins >= r なら相手も r-1 = 未定)。
    /// 先の試合(nextDecided)に結果が付いていれば、勝った側・負けた側どちらでも変えない: 変えると先の結果が別の人に移る。
    /// wins の大きさでは止めない(よそで大きすぎる値が入っても、先が未定なら取り消せる)。
    public static func tap(_ match: Match, side: Side, winsField: String, pending: [String: Int] = [:], nextDecided: Bool) -> TapResult {
        guard match.isPlayable, let a = match.a, let b = match.b else { return .notPlayable }
        let (me, other) = side == .a ? (a, b) : (b, a)
        let r = match.round
        if match.winner != nil, nextDecided { return .locked }
        let wo = wins(other, field: winsField, pending: pending)
        if match.winner == side {
            // 取り消し: 相手も wins >= r(よその値で 2 人とも勝ち)なら一緒に r-1。でないと相手が勝者に変わる
            var out = [me.id: r - 1]
            if wo >= r { out[other.id] = r - 1 }
            return .update(out)
        }
        var out = [me.id: r]
        if wo >= r { out[other.id] = r - 1 }
        return .update(out)
    }
}
