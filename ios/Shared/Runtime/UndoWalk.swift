import MetamonServices

/// 「元に戻す」で戻る先を版の履歴から選ぶ。
/// restore はサーバーで新しい版を作るので「今の版 − 1」だと直前の 2 つを往復する。
/// restore で作られた版は中身の元(`restoredFrom`)に読み替え、その親へ戻る(v5 → v4 → v3 …)。
enum UndoWalk {
    /// 版の中身の元。restore の版は `restoredFrom` を一覧にある限りたどる。
    static func origin(of version: Int, in byVersion: [Int: VersionSummary]) -> Int {
        var v = version
        var seen: Set<Int> = []
        while seen.insert(v).inserted,
              let s = byVersion[v], s.source?.kind == "restore",
              let from = s.source?.restoredFrom, from > 0 {
            v = from
        }
        return v
    }

    /// 戻す先の版。無ければ nil。`versions` は `GET /versions` の結果(並び順は問わない)。
    static func undoTarget(current: Int, versions: [VersionSummary]) -> Int? {
        let byVersion = Dictionary(versions.map { ($0.version, $0) }, uniquingKeysWith: { a, _ in a })
        let from = origin(of: current, in: byVersion)
        let parent: Int?
        if let p = byVersion[from]?.parentVersion, p > 0 {
            parent = p
        } else if byVersion[from] != nil, from == 1 {
            parent = nil
        } else if let listed = versions.map(\.version).filter({ $0 < from }).max() {
            // 親を返さない古い版: 一覧にある 1 つ前の版
            parent = listed
        } else if byVersion[from] == nil, from > 1 {
            // 中身の元が一覧(最新の 1 ページ)より前: 親が分からないので番号で 1 つ前(版は 1 ずつ増える)。「戻せない」で止めない
            parent = from - 1
        } else {
            parent = nil
        }
        guard let parent else { return nil }
        let target = origin(of: parent, in: byVersion)
        return target == from ? nil : target
    }
}
