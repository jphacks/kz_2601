/// 一覧の選んでいる行(Mac・#1387)。Tab は一覧に 1 回だけ止まり、↑↓ で行を移る。
/// 選んでいる行は自前のハイライトで見せる(システムの青いフォーカスリングは出さない)。⌘C・⌫ はこの行が対象。
enum ListRowSelection {
    enum Move { case up, down }

    /// ↑↓ の次の行。何も選んでいない(または消えた)なら ↓ は先頭・↑ は末尾。端では止まる。
    static func moved(from current: String?, _ move: Move, ids: [String]) -> String? {
        guard !ids.isEmpty else { return nil }
        guard let current, let i = ids.firstIndex(of: current) else {
            return move == .down ? ids.first : ids.last
        }
        switch move {
        case .up: return ids[max(i - 1, 0)]
        case .down: return ids[min(i + 1, ids.count - 1)]
        }
    }

    /// 一覧にフォーカスが来た時に選ぶ行: 前に選んでいた行が残っていればそれ、無ければ先頭。
    static func onFocus(current: String?, ids: [String]) -> String? {
        if let current, ids.contains(current) { return current }
        return ids.first
    }

    /// 行が消えたら選択を外す(消えた行を ⌘C・⌫ の対象にしない)。
    static func pruned(_ current: String?, ids: [String]) -> String? {
        guard let current, ids.contains(current) else { return nil }
        return current
    }
}

/// 複数の行を選ぶ(Mac・#1398)。⌘ クリックで 1 行ずつ足し引き、⇧ クリック・⇧↑↓ で範囲。
/// 選んだ行はまとめて ⌘C・⌫・外へのドラッグ(CSV)の対象になる。
struct ListRowMultiSelection: Equatable {
    /// 選んでいる行
    var ids: Set<String> = []
    /// ↑↓・Return・⌘C(1 行の時)の起点。最後にクリック・移動した行
    var cursor: String?
    /// ⇧ で範囲を選ぶ時の起点
    var anchor: String?

    enum Click { case plain, toggle, extend }

    /// 1 行だけを選ぶ(普通のクリック・↑↓)
    mutating func select(_ id: String?) {
        ids = id.map { [$0] } ?? []
        cursor = id
        anchor = id
    }

    /// クリック。`toggle` は ⌘、`extend` は ⇧。`order` は一覧に見えている順の id
    mutating func click(_ id: String, _ kind: Click, order: [String]) {
        switch kind {
        case .plain:
            select(id)
        case .toggle:
            if ids.contains(id) { ids.remove(id) } else { ids.insert(id) }
            cursor = id
            anchor = id
        case .extend:
            ids = Set(Self.range(from: anchor ?? cursor ?? id, to: id, order: order))
            cursor = id
        }
    }

    /// ⇧↑↓: 起点から次の行までを選ぶ
    mutating func extend(_ move: ListRowSelection.Move, order: [String]) {
        guard let next = ListRowSelection.moved(from: cursor, move, ids: order) else { return }
        if anchor == nil || !order.contains(anchor ?? "") { anchor = cursor ?? next }
        ids = Set(Self.range(from: anchor ?? next, to: next, order: order))
        cursor = next
    }

    /// ⌘A(#1035): 一覧の行を全部選ぶ。⌘C で一覧全体を CSV にする時の入口。起点は先頭・カーソルは末尾
    mutating func selectAll(order: [String]) {
        guard !order.isEmpty else { return select(nil) }
        ids = Set(order)
        anchor = order.first
        cursor = order.last
    }

    /// 消えた行を外す
    mutating func prune(order: [String]) {
        let alive = Set(order)
        ids.formIntersection(alive)
        if let c = cursor, !alive.contains(c) { cursor = nil }
        if let a = anchor, !alive.contains(a) { anchor = nil }
    }

    /// 選んでいる行を一覧の順に
    func ordered(_ order: [String]) -> [String] { order.filter(ids.contains) }

    /// 2 行以上を選んでいるか(まとめての操作にするか)
    var isMultiple: Bool { ids.count > 1 }

    /// 1 行だけ選んでいる時のその行(⌘C・⌫・Return の対象)。
    /// `cursor` は ⌘ クリックで外した行や消えた行を指すことがあるので、対象は選んでいる行から決める
    var single: String? { ids.count == 1 ? ids.first : nil }

    static func range(from a: String, to b: String, order: [String]) -> [String] {
        guard let i = order.firstIndex(of: a), let j = order.firstIndex(of: b) else { return order.contains(b) ? [b] : [] }
        return Array(order[min(i, j)...max(i, j)])
    }
}
