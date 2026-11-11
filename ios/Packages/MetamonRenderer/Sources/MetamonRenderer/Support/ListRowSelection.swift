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
