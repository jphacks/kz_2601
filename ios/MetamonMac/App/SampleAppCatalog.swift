import Foundation
import MetamonDSL

struct SampleApp: Identifiable {
    let id: String
    let name: String
    let description: String?
}

/// バンドルに入れた DSL 例(docs/dsl/examples/*.json)を名前順に読む。読めない JSON は飛ばす。
enum SampleAppCatalog {
    static func load(bundle: Bundle = .main) -> [SampleApp] {
        let urls = bundle.urls(forResourcesWithExtension: "json", subdirectory: nil) ?? []
        return urls.compactMap { url in
            guard let dsl = try? AppDSL.load(contentsOf: url) else { return nil }
            return SampleApp(
                id: url.deletingPathExtension().lastPathComponent,
                name: dsl.app.name,
                description: dsl.app.description
            )
        }
        .sorted { $0.name.localizedStandardCompare($1.name) == .orderedAscending }
    }
}
