import Foundation

/// 本体 → コントロール(#1993)の受け渡し: コントロールの設定で選べる生成アプリと記録先の一覧。
/// 設定の画面はウィジェットのプロセスで動き、本体のサーバー・ローカル DB に届かないので、本体が App Group に JSON で 1 件置く。
/// このファイルは Metamon 本体・MetamonWidget・MetamonWidgetTests にコンパイルされる(project.yml の MetamonWidget/Shared)。
struct ControlCatalog: Codable, Equatable {
    struct App: Codable, Equatable, Identifiable {
        var id: String
        var name: String
    }

    /// 記録先(アプリ × entity)。id と見出しは本体の `RecordTargetEntity` と同じ決まり
    struct RecordTarget: Codable, Equatable, Identifiable {
        var appId: String
        var entityId: String
        var appName: String
        var entityName: String
        /// entity が複数あるアプリなら「家計簿 › 支出」と出す
        var showsEntity: Bool

        var id: String { "\(appId)/\(entityId)" }
        var title: String { showsEntity ? "\(appName) › \(entityName)" : appName }
    }

    var apps: [App] = []
    var recordTargets: [RecordTarget] = []

    private static let storageKey = "metamon.control.catalog"

    static func load(from defaults: UserDefaults? = WidgetSnapshot.defaults) -> ControlCatalog {
        guard let data = defaults?.data(forKey: storageKey),
              let catalog = try? JSONDecoder().decode(ControlCatalog.self, from: data) else { return ControlCatalog() }
        return catalog
    }

    /// 変わった時だけ書く。戻り値は書いたか(書いたらコントロールを読み直す)
    @discardableResult
    func save(to defaults: UserDefaults? = WidgetSnapshot.defaults) -> Bool {
        guard self != Self.load(from: defaults), let data = try? JSONEncoder().encode(self) else { return false }
        defaults?.set(data, forKey: Self.storageKey)
        return true
    }

    func app(id: String) -> App? { apps.first { $0.id == id } }
    func recordTarget(id: String) -> RecordTarget? { recordTargets.first { $0.id == id } }

    /// 設定の検索欄。名前の一部(大文字小文字・全角半角を無視)で絞る
    static func matches(_ name: String, _ query: String) -> Bool {
        let q = query.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !q.isEmpty else { return true }
        return name.range(of: q, options: [.caseInsensitive, .widthInsensitive]) != nil
    }
}
