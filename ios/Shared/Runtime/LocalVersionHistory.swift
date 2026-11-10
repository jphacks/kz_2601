import Foundation

/// 生成したアプリの版の「元に戻す / やり直す」を端末で持つ履歴(#209 の端末の履歴・#968)。
/// サーバー(appconfig の versions)に届かない時の戻り先。iOS の `AppRuntime` の `localHistory` / `RedoPoint` と同じ決まり:
/// - 新しい版が当たるたびに前の版を覚える(直近 `limit` 件)。新しい版が当たると「やり直す」は消える
/// - 戻すと、戻す前の版を「やり直す」の行き先にする(1 つだけ)
/// - やり直すと、その前の版をまた戻り先に足す
struct LocalVersionHistory<Version> {
    static var limit: Int { 10 }

    private(set) var past: [Version] = []
    /// 「やり直す」の行き先(0 か 1 件)。`Version` が Optional でも区別できるよう配列で持つ
    private var future: [Version] = []

    init() {}

    var canUndo: Bool { !past.isEmpty }
    var canRedo: Bool { !future.isEmpty }

    /// 新しい版が当たった。`previous` = 当たる前の版
    mutating func record(previous: Version) {
        remember(previous)
        future.removeAll()
    }

    /// 戻す。`current` = 今の版。戻り先を返す(無ければ nil で何も変えない)
    mutating func undo(from current: Version) -> Version? {
        guard let previous = past.popLast() else { return nil }
        future = [current]
        return previous
    }

    /// やり直す。`current` = 今の(戻した)版。行き先を返す(無ければ nil で何も変えない)
    mutating func redo(from current: Version) -> Version? {
        guard let next = future.popLast() else { return nil }
        remember(current)
        return next
    }

    mutating func reset() {
        past.removeAll()
        future.removeAll()
    }

    private mutating func remember(_ version: Version) {
        past.append(version)
        if past.count > Self.limit { past.removeFirst(past.count - Self.limit) }
    }
}
