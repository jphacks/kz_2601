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
}
