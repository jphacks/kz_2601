import Foundation
import MetamonDSL

/// 画面マップ(#116)をキーボードでたどる・マップから画面を開く(Mac #970)ための純粋関数。
public extension ScreenGraph {
    /// 矢印キーの向き
    enum Move: Sendable {
        case up, down, left, right
    }

    /// 上の段から順に、段の中は左から(VoiceOver とキーボードの読む順)
    var readingOrder: [String] { rows.flatMap { $0 } }

    /// 矢印キーで選ぶ隣のカード。何も選んでいなければ先頭。端なら nil。
    /// 段は中央揃えで並ぶので、上下は段の中心からのずれがいちばん近いカード(同じなら左)。
    func neighbor(of id: String?, _ move: Move) -> String? {
        guard let id, let node = node(id), rows.indices.contains(node.row) else { return rows.first?.first }
        let row = rows[node.row]
        switch move {
        case .left:
            return node.column > 0 ? row[node.column - 1] : nil
        case .right:
            return node.column + 1 < row.count ? row[node.column + 1] : nil
        case .up, .down:
            let r = node.row + (move == .up ? -1 : 1)
            guard rows.indices.contains(r) else { return nil }
            let x = Self.offset(node.column, count: row.count)
            let target = rows[r]
            let best = target.indices.min { abs(Self.offset($0, count: target.count) - x) < abs(Self.offset($1, count: target.count) - x) }
            return best.map { target[$0] }
        }
    }

    private static func offset(_ column: Int, count: Int) -> Double {
        Double(column) - Double(count - 1) / 2
    }

    /// 根からこの画面までの最短の道(矢印の列・BFS で段を決めた時と同じ順にたどる)。根なら空、行けない・無い画面は nil。
    func path(to id: String) -> [Link]? {
        guard let target = node(id), target.reachable else { return nil }
        var parent: [String: Link] = [:]
        var frontier = nodes.filter { $0.depth == 0 }.map(\.id)
        var seen = Set(frontier)
        while target.depth != 0, parent[id] == nil, !frontier.isEmpty {
            var found: [String] = []
            for from in frontier {
                for link in outgoing(from: from) where !seen.contains(link.to) {
                    seen.insert(link.to)
                    parent[link.to] = link
                    found.append(link.to)
                }
            }
            frontier = found
        }
        var steps: [Link] = []
        var cursor = id
        while let link = parent[cursor] {
            steps.insert(link, at: 0)
            cursor = link.from
        }
        return steps
    }
}

public extension NavigationResolver {
    /// 画面マップから画面を開く(Mac #970)。開いているモーダルは閉じ、根(タブ)から最短の道を `navigate` でたどる。
    /// たどる矢印は先頭の辺の出し方(push / sheet / fullScreen)で開くので、戻る・閉じるはアプリの中で押した時と同じに効く。
    /// 記録は選ばずに開く(フォームは新規・詳細は空)。根から行けない画面は今のタブ(無ければ main)に積む。無い画面は nil。
    static func reveal(_ screen: String, graph: ScreenGraph, dsl: AppDSL, from current: NavigationState) -> NavigationState? {
        guard graph.node(screen) != nil else { return nil }
        var s = dismissModals(current)
        guard let steps = graph.path(to: screen) else {
            let root = mainRoot(state: s, dsl: dsl)
            s.paths[root.stack] = [Route(screen: screen, stack: root.stack)]
            return s
        }
        let start = steps.first?.from ?? screen
        if dsl.app.hasTabs {
            s.selectedTab = start
            s.paths[Route.tabStack(start)] = []
        } else {
            s.paths["main"] = []
        }
        for link in steps {
            s = navigate(s, to: link.to, presentation: link.edges.first?.presentation, recordId: nil, from: top(s, dsl: dsl), dsl: dsl)
        }
        return s
    }

    /// 今いちばん上に見えている画面(フルスクリーン → シート → 選択中タブ / main の順)
    static func top(_ s: NavigationState, dsl: AppDSL) -> Route {
        if let full = s.fullScreen { return s.paths["full"]?.last ?? full }
        if let sheet = s.sheet { return s.paths["sheet"]?.last ?? sheet }
        let root = mainRoot(state: s, dsl: dsl)
        return s.paths[root.stack]?.last ?? root
    }
}
