import Foundation

/// サイドバーの「作ったアプリ」の並び(#1057)。サーバーの一覧と、この Mac にだけあるアプリを混ぜる。
enum MacServerAppList {
    /// この Mac にだけあるアプリ(端末で複製した・モックで作った・作った直後で一覧に載る前)を今の並びで上に、
    /// 続けてサーバーの並び(iOS の変身一覧と同じ)。サーバーの一覧に無いアプリも端末から消さない
    static func merged(local: [String], server: [String]) -> [String] {
        var seen = Set<String>()
        let serverIds = Set(server)
        return (local.filter { !serverIds.contains($0) } + server).filter { seen.insert($0).inserted }
    }
}
