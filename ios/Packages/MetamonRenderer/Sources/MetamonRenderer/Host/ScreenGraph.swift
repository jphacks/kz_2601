import Foundation
import MetamonDSL

/// 画面マップ(#116)の元になる画面グラフ。backend `pkg/dsl/nav.go` の NavEdges / NavRoots / Reachable と同じ考え方の純粋関数。
/// - 辺: button / toolbar / list の行タップ / calendar の日付タップの `navigate` と、`saveRecord` の保存後遷移先(`screen`)
/// - 根: タブがあればタブの根(タブ順)、無ければ initialScreen。タブの有無は iOS の描画と同じ `hasTabs`(2 個以上)で決める
/// - 段: 根を 0 段目に、根からの BFS の深さで段を決める。同じ段の中は見つかった順(親の並び順)。到達できない画面は最後の段に文書順
public struct ScreenGraph: Equatable, Sendable {
    /// 遷移のきっかけ(図の詳細で「どこから行けるか」を言葉にする)。
    public enum Trigger: Equatable, Sendable {
        case button(String)
        case toolbar(String)
        case itemTap
        case dateTap
        /// saveRecord の保存後に開く
        case afterSave
        /// button 以外の部品の action
        case other
    }

    /// 1 本の遷移(nav.go の NavEdge と 1:1。存在しない画面への辺も含む)。
    public struct Edge: Equatable, Sendable {
        public var from: String
        public var to: String
        public var presentation: Presentation
        public var trigger: Trigger

        public var isModal: Bool { presentation != .push }
    }

    /// 図に描く矢印。同じ from→to の辺を 1 本にまとめる(両端とも存在する画面の時だけ)。
    public struct Link: Equatable, Sendable, Identifiable {
        public var id: String { "\(from)->\(to)" }
        public var from: String
        public var to: String
        public var edges: [Edge]
        /// まとめた辺がすべて sheet / fullScreen なら破線で描く
        public var isModal: Bool { edges.allSatisfy(\.isModal) }
    }

    public struct Node: Equatable, Sendable, Identifiable {
        public var id: String
        public var title: String
        public var isRoot: Bool
        /// フォーム画面(screen.entity あり)
        public var isForm: Bool
        /// 根からの BFS の深さ。到達できなければ nil
        public var depth: Int?
        public var reachable: Bool { depth != nil }
        /// レイアウト上の段と段内の位置(0 始まり)
        public var row: Int
        public var column: Int
    }

    public var nodes: [Node]
    public var edges: [Edge]
    public var links: [Link]
    /// 段ごとの画面 id(上から)。到達できない画面があれば最後の段にまとまる
    public var rows: [[String]]

    public init(dsl: AppDSL) {
        let edges = Self.edges(of: dsl)
        let ids = dsl.screens.map(\.id)
        let exists = Set(ids)
        var next: [String: [String]] = [:]
        for e in edges where exists.contains(e.to) && !(next[e.from]?.contains(e.to) ?? false) {
            next[e.from, default: []].append(e.to)
        }

        // BFS(根の順 → 見つかった順)で段を決める
        let roots = Self.roots(of: dsl)
        var depth: [String: Int] = [:]
        var rows: [[String]] = []
        var frontier: [String] = []
        for r in roots where exists.contains(r) && depth[r] == nil {
            depth[r] = 0
            frontier.append(r)
        }
        while !frontier.isEmpty {
            rows.append(frontier)
            var found: [String] = []
            for id in frontier {
                for to in next[id] ?? [] where depth[to] == nil {
                    depth[to] = rows.count
                    found.append(to)
                }
            }
            frontier = found
        }
        let unreachable = ids.filter { depth[$0] == nil }
        if !unreachable.isEmpty { rows.append(unreachable) }

        var place: [String: (row: Int, column: Int)] = [:]
        for (r, row) in rows.enumerated() {
            for (c, id) in row.enumerated() { place[id] = (r, c) }
        }
        let rootSet = Set(roots)
        self.nodes = dsl.screens.map { s in
            Node(id: s.id, title: s.title.isEmpty ? s.id : s.title, isRoot: rootSet.contains(s.id), isForm: s.entity != nil,
                 depth: depth[s.id], row: place[s.id]?.row ?? 0, column: place[s.id]?.column ?? 0)
        }
        self.edges = edges
        var links: [Link] = []
        for e in edges where exists.contains(e.from) && exists.contains(e.to) {
            if let i = links.firstIndex(where: { $0.from == e.from && $0.to == e.to }) {
                links[i].edges.append(e)
            } else {
                links.append(Link(from: e.from, to: e.to, edges: [e]))
            }
        }
        self.links = links
        self.rows = rows
    }

    public func node(_ id: String) -> Node? { nodes.first { $0.id == id } }
    /// この画面から出ていく矢印
    public func outgoing(from id: String) -> [Link] { links.filter { $0.from == id } }
    /// この画面へ入ってくる矢印
    public func incoming(to id: String) -> [Link] { links.filter { $0.to == id } }
    /// 根から行けない画面(文書順)
    public var unreachable: [String] { nodes.filter { !$0.reachable }.map(\.id) }

    /// 根(nav.go の NavRoots)。タブがあればタブの根を重複なくタブ順、無ければ initialScreen。
    public static func roots(of dsl: AppDSL) -> [String] {
        guard dsl.app.hasTabs else { return [dsl.initialScreen] }
        var out: [String] = []
        for t in dsl.app.tabs where !out.contains(t.screen) { out.append(t.screen) }
        return out
    }

    /// 全遷移を画面順に列挙する(nav.go の NavEdges と同じ走査順: toolbar → components(子も)の action / onItemTap / onDateTap)。
    public static func edges(of dsl: AppDSL) -> [Edge] {
        var out: [Edge] = []
        for s in dsl.screens {
            func add(_ action: Action?, _ trigger: Trigger) {
                guard let action, let to = action.screen, !to.isEmpty else { return }
                switch action.type {
                case .navigate:
                    out.append(Edge(from: s.id, to: to, presentation: action.presentation ?? .push, trigger: trigger))
                case .saveRecord:
                    out.append(Edge(from: s.id, to: to, presentation: action.presentation ?? .push, trigger: .afterSave))
                default:
                    break
                }
            }
            for t in DuplicateAddControl.toolbar(of: s, in: dsl) { add(t.action, .toolbar(t.label)) } // 画面に出ない重複の＋は数えない(#416)
            for c in s.allComponents {
                add(c.action, c.type == .button ? .button(c.label ?? c.text ?? "") : .other)
                add(c.onItemTap, .itemTap)
                add(c.onDateTap, .dateTap)
            }
        }
        return out
    }
}
