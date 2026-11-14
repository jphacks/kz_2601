import Foundation
import MetamonDSL

struct SampleApp: Identifiable, Hashable {
    let id: String
    let name: String
    let description: String?
    /// 同梱の DSL 例の場所。会話で作った・直したアプリ(#967)は nil で、DSL は `MacGeneratedApps` が持つ
    var url: URL?
    /// 会話で直すたびに 1 つ上がる(中央のランタイムを作り直す目印)。同梱の例は 0
    var version = 0

    func loadDSL() throws -> AppDSL {
        guard let url else { throw CocoaError(.fileNoSuchFile) }
        return try AppDSL.load(contentsOf: url)
    }

    /// サイドバーの検索(#1282)。名前と説明に、空白で区切った語がすべて含まれるか(大文字小文字・かなの幅は区別しない)。空なら全部
    func matches(_ query: String) -> Bool {
        let words = query.split(whereSeparator: \.isWhitespace)
        let haystack = [name, description ?? ""].joined(separator: "\n")
        return words.allSatisfy { haystack.localizedStandardContains($0) }
    }
}

/// バンドルに入れた DSL 例(docs/dsl/examples/*.json)を名前順に読む。読めない JSON は飛ばす。
/// サーバーのアプリの一覧に置き換えるのは後の Issue。
enum SampleAppCatalog {
    static func load(bundle: Bundle = .main) -> [SampleApp] {
        let urls = bundle.urls(forResourcesWithExtension: "json", subdirectory: nil) ?? []
        return urls.compactMap { url in
            guard let dsl = try? AppDSL.load(contentsOf: url) else { return nil }
            return SampleApp(
                id: url.deletingPathExtension().lastPathComponent,
                name: dsl.app.name,
                description: dsl.app.description,
                url: url
            )
        }
        .sorted { $0.name.localizedStandardCompare($1.name) == .orderedAscending }
    }

    /// サイドバーの「同梱の例」に出す例(展示向け)。Mac で動き、1 人ですぐ触れて見栄えのするものをこの順で並べる。
    /// 開発用(gallery-*・画面遷移の確認・はじめてのアプリ)と Mac で使えない部品の例(ドラレコ・ヘルスケア・センサー・歩いた道)は出さない。
    /// 出さない例もバンドルには残す(ショートカット・UI テストが id で開く)
    static let showcaseIds = ["kakeibo", "shopping", "medicine", "water", "timetable", "vocab", "warikan", "tournament"]

    /// `apps` から展示する例を `showcaseIds` の順で選ぶ。`keeping` の例(開いている例)は出さない例でも末尾に残す
    static func showcase(_ apps: [SampleApp], keeping keep: String? = nil) -> [SampleApp] {
        let byId = Dictionary(apps.map { ($0.id, $0) }, uniquingKeysWith: { first, _ in first })
        var picked = showcaseIds.compactMap { byId[$0] }
        if let keep, !showcaseIds.contains(keep), let app = byId[keep] { picked.append(app) }
        return picked
    }
}
