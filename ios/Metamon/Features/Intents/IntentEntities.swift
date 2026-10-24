import AppIntents
import MetamonDSL
import MetamonServices

// MARK: - 作ったアプリ

/// 変身一覧のアプリ 1 件。「Tailor It で家計簿を開く」の `家計簿`。
/// DSL は変えない: アプリを作れば `GET /v1/apps` に出て、そのままここに出る。
struct TailorAppEntity: AppEntity {
    static var typeDisplayRepresentation: TypeDisplayRepresentation = "アプリ"
    static var defaultQuery = TailorAppQuery()

    var id: String
    var name: String

    var displayRepresentation: DisplayRepresentation { DisplayRepresentation(title: "\(name)") }

    init(_ app: AppSummary) {
        id = app.id
        name = app.name
    }
}

struct TailorAppQuery: EntityStringQuery {
    @MainActor
    func entities(for identifiers: [String]) async throws -> [TailorAppEntity] {
        let apps = await IntentHost.shared.apps()
        return identifiers.compactMap { id in apps.first { $0.id == id }.map(TailorAppEntity.init) }
    }

    @MainActor
    func suggestedEntities() async throws -> [TailorAppEntity] {
        await IntentHost.shared.apps().map(TailorAppEntity.init)
    }

    /// 「家計簿アプリ」「かけいぼ」のような言い方も変身一覧の音声切替と同じ規則(`AppNameMatch`)で当てる。
    @MainActor
    func entities(matching string: String) async throws -> [TailorAppEntity] {
        let apps = await IntentHost.shared.apps()
        return IntentMatching.indices(of: string, in: apps.map(\.name)).map { TailorAppEntity(apps[$0]) }
    }
}

// MARK: - 記録先(アプリ × entity)

/// 「記録を追加」の行き先。アプリと entity を 1 つにまとめる(パラメータ依存の連鎖を作らず、Siri の聞き返しを 1 回で済ませる)。
/// entity が 1 つのアプリは「家計簿」、複数なら「家計簿 › 支出」と出す。
struct RecordTargetEntity: AppEntity {
    static var typeDisplayRepresentation: TypeDisplayRepresentation = "記録先"
    static var defaultQuery = RecordTargetQuery()

    /// `<appId>/<entityId>`
    var id: String
    var appId: String
    var entityId: String
    var appName: String
    var entityName: String
    var showsEntity: Bool

    var title: String { showsEntity ? "\(appName) › \(entityName)" : appName }

    var displayRepresentation: DisplayRepresentation {
        DisplayRepresentation(title: "\(title)", subtitle: showsEntity ? nil : "\(entityName)")
    }

    init(app: AppSummary, entity: Entity, showsEntity: Bool) {
        id = "\(app.id)/\(entity.id)"
        appId = app.id
        entityId = entity.id
        appName = app.name
        entityName = entity.name
        self.showsEntity = showsEntity
    }

    /// 全アプリの記録先(フォームのある entity を先に)。DSL はキャッシュ優先で、未取得のアプリだけ `GET /dsl` する。
    @MainActor
    static func all(limit: Int = 12) async -> [RecordTargetEntity] {
        var out: [RecordTargetEntity] = []
        for app in await IntentHost.shared.apps().prefix(limit) {
            guard let doc = await IntentHost.shared.document(appId: app.id) else { continue }
            let entities = QuickRecord.recordableEntities(in: doc.dsl)
            out += entities.map { RecordTargetEntity(app: app, entity: $0, showsEntity: entities.count > 1) }
        }
        return out
    }
}

struct RecordTargetQuery: EntityStringQuery {
    @MainActor
    func entities(for identifiers: [String]) async throws -> [RecordTargetEntity] {
        let all = await RecordTargetEntity.all()
        return identifiers.compactMap { id in all.first { $0.id == id } }
    }

    @MainActor
    func suggestedEntities() async throws -> [RecordTargetEntity] {
        await RecordTargetEntity.all()
    }

    @MainActor
    func entities(matching string: String) async throws -> [RecordTargetEntity] {
        let all = await RecordTargetEntity.all()
        let byApp = IntentMatching.indices(of: string, in: all.map(\.appName))
        let hits = byApp.isEmpty ? IntentMatching.indices(of: string, in: all.map(\.entityName)) : byApp
        return hits.map { all[$0] }
    }
}

/// 名前照合(変身一覧の音声切替と同じ `AppNameMatch`)。曖昧なら候補を全部返し、Siri に選ばせる。
enum IntentMatching {
    static func indices(of query: String, in names: [String]) -> [Int] {
        switch AppNameMatch.match(query, in: names) {
        case .one(let i):
            // 同名(entity が複数あるアプリ)はまとめて返す
            return names.indices.filter { names[$0] == names[i] }
        case .ambiguous(let list): return list
        case .none: return []
        }
    }
}
