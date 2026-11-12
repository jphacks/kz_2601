import AppIntents
import MetamonDSL
import MetamonServices

// ショートカットで生成アプリの数字(stat)を読む(#1994)。iOS だけ(Mac は #1034 の後で足す)。
// 数え方はウィジェット(`WidgetSnapshotWriter.statText`)と同じ関数なので、ウィジェット・アプリの中の stat と値が一致する。

// MARK: - 数字(アプリ × stat)

/// 「家計簿 › 今月の支出」。id は `<appId>/<stat の key>`(`WidgetSnapshotWriter.statKey`・id を書かない stat でも変わらない)。
struct StatEntity: AppEntity {
    static var typeDisplayRepresentation: TypeDisplayRepresentation = "数字"
    static var defaultQuery = StatQuery()

    var id: String
    var appId: String
    var statKey: String
    var appName: String
    var label: String
    /// アプリか stat が消えた(実行時に分かる文で失敗させるため、id だけで残す)
    var isMissing = false

    var title: String { isMissing ? String(localized: "見つからない数字") : "\(appName) › \(label)" }

    var displayRepresentation: DisplayRepresentation { DisplayRepresentation(title: "\(title)") }

    init(app: AppSummary, stat: Component, dsl: AppDSL) {
        appId = app.id
        statKey = WidgetSnapshotWriter.statKey(stat)
        id = Self.id(appId: appId, statKey: statKey)
        appName = app.name
        label = StatReading.label(of: stat, in: dsl)
    }

    init(missingId id: String) {
        self.id = id
        (appId, statKey) = Self.split(id) ?? (id, "")
        appName = ""
        label = ""
        isMissing = true
    }

    static func id(appId: String, statKey: String) -> String { "\(appId)/\(statKey)" }

    static func split(_ id: String) -> (appId: String, statKey: String)? {
        guard let slash = id.firstIndex(of: "/") else { return nil }
        return (String(id[..<slash]), String(id[id.index(after: slash)...]))
    }

    /// 全アプリの数字(画面順)。DSL はキャッシュ優先で、未取得のアプリだけ `GET /dsl` する(`RecordTargetEntity.all` と同じ作り)。
    @MainActor
    static func all(limit: Int = 12) async -> [StatEntity] {
        var out: [StatEntity] = []
        for app in await IntentHost.shared.apps().prefix(limit) {
            guard let doc = await IntentHost.shared.document(appId: app.id) else { continue }
            out += StatReading.stats(in: doc.dsl).map { StatEntity(app: app, stat: $0, dsl: doc.dsl) }
        }
        return out
    }
}

struct StatQuery: EntityStringQuery {
    /// 選んだ id だけ引き直す。消えたものは `isMissing` で返し、実行時に「見つかりません」と言う(黙って別の数字を出さない)
    @MainActor
    func entities(for identifiers: [String]) async throws -> [StatEntity] {
        let apps = await IntentHost.shared.apps()
        var out: [StatEntity] = []
        for id in identifiers {
            guard let (appId, key) = StatEntity.split(id), let app = apps.first(where: { $0.id == appId }),
                  let doc = await IntentHost.shared.document(appId: appId),
                  let stat = StatReading.stat(key, in: doc.dsl) else {
                out.append(StatEntity(missingId: id))
                continue
            }
            out.append(StatEntity(app: app, stat: stat, dsl: doc.dsl))
        }
        return out
    }

    @MainActor
    func suggestedEntities() async throws -> [StatEntity] {
        await StatEntity.all()
    }

    @MainActor
    func entities(matching string: String) async throws -> [StatEntity] {
        let all = await StatEntity.all()
        let byApp = IntentMatching.indices(of: string, in: all.map(\.appName))
        let hits = byApp.isEmpty ? IntentMatching.indices(of: string, in: all.map(\.label)) : byApp
        return hits.map { all[$0] }
    }
}

// MARK: - 数える(ウィジェットと同じ)

enum StatReading {
    struct Value: Equatable, Sendable {
        /// stat と同じ書式の文字列(「¥12,340」「3件」)
        var text: String
        /// 数で取れるもの(合計・平均・件数・数の式)。latest / earliest・文字の式は nil
        var number: Double?
    }

    /// 選べる数字(ウィジェット・一覧のひと目の数字と同じ並び)
    static func stats(in dsl: AppDSL) -> [Component] { AppGlance.choices(in: dsl) }

    static func stat(_ key: String, in dsl: AppDSL) -> Component? {
        stats(in: dsl).first { WidgetSnapshotWriter.statKey($0) == key }
    }

    /// 見出し。label が無ければ entity 名、それも無ければ「数字」
    static func label(of c: Component, in dsl: AppDSL) -> String {
        if let label = c.label, !label.isEmpty { return label }
        if let name = c.entity.flatMap(dsl.entity)?.name, !name.isEmpty { return name }
        return String(localized: "数字")
    }

    /// 数えるのに読む entity(`AppGlance.entities` と同じ)
    static func entities(for c: Component, in dsl: AppDSL) -> [String] { AppGlance.entities(for: c, in: dsl) }

    /// 値。記録が未取得なら nil(ウィジェットの `statText` と同じ判定)
    static func read(_ c: Component, dsl: AppDSL, records: [String: [Record]], countUnit: String) -> Value? {
        let evaluator = ExprEvaluator(dsl: dsl, records: { records[$0] ?? [] })
        guard let text = WidgetSnapshotWriter.statText(c, dsl: dsl, records: records, evaluator: evaluator, countUnit: countUnit) else {
            return nil
        }
        return Value(text: text, number: number(c, dsl: dsl, records: records, evaluator: evaluator))
    }

    /// `WidgetSnapshotWriter.formatted` と同じ絞り込み・集計の、書式を掛ける前の数
    static func number(_ c: Component, dsl: AppDSL, records: [String: [Record]], evaluator: ExprEvaluator) -> Double? {
        if c.aggregate == nil, let expr = c.valueExpr {
            if case .number(let n)? = evaluator.evaluate(expr) { return n }
            return nil
        }
        guard !Aggregator.isPick(c.aggregate), let entityId = c.entity, records[entityId] != nil else { return nil }
        let rows = Aggregator.apply(c.filter, entity: dsl.entity(entityId), to: evaluator.viewRecords(for: entityId))
        return Aggregator.aggregate(c.aggregate, field: c.field, over: rows)
    }
}

// MARK: - 数字を読む

/// 結果。端末に記録が無い時は一覧のひと目の数字(`AppGlanceStore`)の値、それも無ければ未読み込み
enum StatLookup: Equatable {
    case value(StatReading.Value)
    case cached(String)
    case notLoaded
    /// 暗号化した field を数える stat。値は返さない(アプリの中でだけ見られる)
    case locked
}

@MainActor
enum StatLookupRunner {
    /// アプリか stat が消えていたら `statNotFound`。ネットに出られなくても落ちない(DSL・記録は端末のキャッシュ)
    static func run(_ target: StatEntity) async throws -> (stat: StatEntity, result: StatLookup) {
        guard !target.isMissing, let doc = await IntentHost.shared.document(appId: target.appId),
              let c = StatReading.stat(target.statKey, in: doc.dsl) else { throw TailorIntentError.statNotFound }
        var stat = target
        stat.label = StatReading.label(of: c, in: doc.dsl)
        let result = await resolve(c, dsl: doc.dsl, glance: AppGlanceStore.shared.values[target.appId]) {
            await IntentHost.shared.statRecords(appId: target.appId, entities: StatReading.entities(for: c, in: doc.dsl),
                                                isExpression: c.aggregate == nil && c.valueExpr != nil)
        }
        return (stat, result)
    }

    /// 値を決める。暗号化した field の stat は記録を読まずに `.locked`(一覧のひと目の数字の「🔒」と同じ判定)。
    /// 記録で数えられなければ一覧のひと目の数字(古くても無いよりよい)、それも無ければ未読み込み
    static func resolve(_ c: Component, dsl: AppDSL, glance: AppGlance.Value?,
                        records: () async -> [String: [Record]]?) async -> StatLookup {
        if AppGlance.isEncrypted(c, in: dsl) { return .locked }
        if let records = await records(),
           let value = StatReading.read(c, dsl: dsl, records: records, countUnit: WidgetSnapshotWriter.countUnit) {
            return .value(value)
        }
        if let text = cachedGlance(glance, for: c) { return .cached(text) }
        return .notLoaded
    }

    /// 一覧のひと目の数字のうち、この stat のもの。`statKey` で突き合わせる(id を書かない stat は読むたびに id が変わる)
    nonisolated static func cachedGlance(_ glance: AppGlance.Value?, for c: Component) -> String? {
        guard let glance, glance.statKey == WidgetSnapshotWriter.statKey(c) else { return nil }
        return glance.value
    }
}

/// 「Tailor It で数字を読む」: アプリを開かずに stat の値(表示の文字列)を返す。Siri は「今月の支出は ¥12,340 です」
struct GetStatValueIntent: AppIntent {
    static var title: LocalizedStringResource = "数字を読む"
    static var description = IntentDescription("Tailor It を開かずに、記録の集計(ウィジェットと同じ数字)を読みます。例: 家計簿の今月の支出。")
    static var openAppWhenRun = false
    /// 記録の集計を返すので、ロック中は端末のロックを解いてから
    static var authenticationPolicy: IntentAuthenticationPolicy = .requiresAuthentication

    @Parameter(title: "数字", requestValueDialog: "どの数字を読みますか?")
    var target: StatEntity

    static var parameterSummary: some ParameterSummary {
        Summary("\(\.$target) を読む")
    }

    @MainActor
    func perform() async throws -> some IntentResult & ReturnsValue<String> & ProvidesDialog {
        let (stat, result) = try await StatLookupRunner.run(target)
        switch result {
        case .value(let v): return .result(value: v.text, dialog: IntentDialog(Self.valueMessage(label: stat.label, value: v.text)))
        case .cached(let text): return .result(value: text, dialog: IntentDialog(Self.valueMessage(label: stat.label, value: text)))
        case .notLoaded:
            return .result(value: String(localized: Self.notLoadedValue),
                           dialog: IntentDialog(Self.notLoadedMessage(appName: stat.appName)))
        case .locked:
            return .result(value: String(localized: Self.lockedValue), dialog: IntentDialog(Self.lockedMessage(label: stat.label)))
        }
    }

    static func valueMessage(label: String, value: String) -> LocalizedStringResource { "\(label)は\(value)です" }

    static let notLoadedValue: LocalizedStringResource = "まだ読み込んでいません"

    static func notLoadedMessage(appName: String) -> LocalizedStringResource {
        "まだ読み込んでいません。Tailor It で\(appName)を一度開いてください"
    }

    static let lockedValue: LocalizedStringResource = "ロック中です。Tailor It を開いて確認してください"

    static func lockedMessage(label: String) -> LocalizedStringResource {
        "「\(label)」はロック中です。Tailor It を開いて確認してください"
    }
}

/// 「数字を読む(数値)」: 計算に使えるよう Double で返す。latest / earliest・文字の式・未読み込みは分かる文で失敗する
struct GetStatNumberIntent: AppIntent {
    static var title: LocalizedStringResource = "数字を読む(数値)"
    static var description = IntentDescription("Tailor It を開かずに、記録の集計を数値で読みます。計算や比較に使えます。")
    static var openAppWhenRun = false
    static var authenticationPolicy: IntentAuthenticationPolicy = .requiresAuthentication

    @Parameter(title: "数字", requestValueDialog: "どの数字を読みますか?")
    var target: StatEntity

    static var parameterSummary: some ParameterSummary {
        Summary("\(\.$target) を数値で読む")
    }

    @MainActor
    func perform() async throws -> some IntentResult & ReturnsValue<Double> {
        let (stat, result) = try await StatLookupRunner.run(target)
        switch result {
        case .value(let v):
            guard let n = v.number else { throw TailorIntentError.statNotNumber(stat.label) }
            return .result(value: n)
        case .cached, .notLoaded: throw TailorIntentError.statNotLoaded(stat.appName)
        case .locked: throw TailorIntentError.statLocked(stat.label)
        }
    }
}
