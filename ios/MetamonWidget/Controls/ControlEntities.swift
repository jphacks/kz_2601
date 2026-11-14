import AppIntents

// コントロールの設定(#1993)で選ぶもの。設定はウィジェットのプロセスで動くので、本体の TailorAppEntity / RecordTargetEntity
// (Query が本体のサーバー・DB を読む)は使えない。中身(id・見出し)は同じで、一覧は本体が App Group に書いた `ControlCatalog` から読む。

/// 生成アプリ 1 件(本体の `TailorAppEntity` と同じ id・名前)
struct ControlAppEntity: AppEntity {
    #if APP_STORE_REVIEW // 審査用の版は記録帳と言う(#247)
    static var typeDisplayRepresentation: TypeDisplayRepresentation = "記録帳"
    #else
    static var typeDisplayRepresentation: TypeDisplayRepresentation = "アプリ"
    #endif
    static var defaultQuery = ControlAppQuery()

    var id: String
    var name: String

    var displayRepresentation: DisplayRepresentation { DisplayRepresentation(title: "\(name)") }

    init(_ app: ControlCatalog.App) {
        id = app.id
        name = app.name
    }
}

struct ControlAppQuery: EntityStringQuery {
    func entities(for identifiers: [String]) async throws -> [ControlAppEntity] {
        let catalog = ControlCatalog.load()
        return identifiers.compactMap { catalog.app(id: $0).map(ControlAppEntity.init) }
    }

    func suggestedEntities() async throws -> [ControlAppEntity] {
        ControlCatalog.load().apps.map(ControlAppEntity.init)
    }

    func entities(matching string: String) async throws -> [ControlAppEntity] {
        ControlCatalog.load().apps.filter { ControlCatalog.matches($0.name, string) }.map(ControlAppEntity.init)
    }
}

/// 記録先 1 件(本体の `RecordTargetEntity` と同じ `<appId>/<entityId>`・見出し)
struct ControlRecordTargetEntity: AppEntity {
    static var typeDisplayRepresentation: TypeDisplayRepresentation = "記録先"
    static var defaultQuery = ControlRecordTargetQuery()

    var id: String
    var appId: String
    var entityId: String
    var title: String

    var displayRepresentation: DisplayRepresentation { DisplayRepresentation(title: "\(title)") }

    init(_ target: ControlCatalog.RecordTarget) {
        id = target.id
        appId = target.appId
        entityId = target.entityId
        title = target.title
    }
}

struct ControlRecordTargetQuery: EntityStringQuery {
    func entities(for identifiers: [String]) async throws -> [ControlRecordTargetEntity] {
        let catalog = ControlCatalog.load()
        return identifiers.compactMap { catalog.recordTarget(id: $0).map(ControlRecordTargetEntity.init) }
    }

    func suggestedEntities() async throws -> [ControlRecordTargetEntity] {
        ControlCatalog.load().recordTargets.map(ControlRecordTargetEntity.init)
    }

    func entities(matching string: String) async throws -> [ControlRecordTargetEntity] {
        ControlCatalog.load().recordTargets.filter { ControlCatalog.matches($0.title, string) }.map(ControlRecordTargetEntity.init)
    }
}
