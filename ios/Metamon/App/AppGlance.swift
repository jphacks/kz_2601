import Foundation
import Observation
import MetamonDSL
import MetamonServices

/// アプリの一覧のカードに出す「ひと目の数字」(link-10・例「今月 ¥38,200」)。
/// 数え方はウィジェット(`WidgetSnapshotWriter`)と同じなので、アプリの中の stat と数字が一致する。
/// 数える処理(`compute(_:)`)は main actor の外で走る(1 entity 最大 1000 行・一覧を開いた時に引っかからない)。
enum AppGlance {
    /// `AppSummary.glanceStat` の「出さない」。
    static let hidden = "-"
    /// これより古い値は薄く出す。
    static let staleAfter: TimeInterval = 60 * 60

    struct Value: Codable, Equatable, Sendable {
        var statId: String
        var label: String
        var value: String
        var at: Date
        /// `WidgetSnapshotWriter.statKey`。id を書かない stat は読むたびに id が変わるので、ショートカットはこちらで突き合わせる(#1994)。古い保存値は nil
        var statKey: String? = nil
    }

    /// 数えるのに要るものの写し。言語の設定(件数の単位)は main actor で引いて入れる。
    struct Input: Sendable {
        var dsl: AppDSL
        var records: [String: [Record]]
        var setting: String?
        var countUnit: String
        var now: Date
    }

    /// 数える処理(テストで差し替えて、main の外で走ることを確かめる)。
    typealias Compute = @Sendable (Input) -> Value?

    /// 選べる数字(画面順の stat・ウィジェットの `statComponents` と同じ)。初期状態のアプリは無し。
    static func choices(in dsl: AppDSL) -> [Component] {
        dsl.isVoiceOnly ? [] : WidgetSnapshotWriter.statComponents(in: dsl)
    }

    /// 出す stat。既定(nil・空・DSL に無い id)は先頭、"-" は出さない。
    static func chosen(in dsl: AppDSL, setting: String?) -> Component? {
        guard setting != hidden else { return nil }
        let all = choices(in: dsl)
        if let setting, !setting.isEmpty, let c = all.first(where: { $0.id == setting }) { return c }
        return all.first
    }

    /// 数えるのに読む entity(式の stat は全 entity)。
    static func entities(for c: Component, in dsl: AppDSL) -> [String] {
        if c.aggregate == nil, c.valueExpr != nil { return dsl.entities.map(\.id) }
        return c.entity.map { [$0] } ?? []
    }

    /// 暗号化した項目を使う stat に出す印。値は端末に残さない。
    static let locked = "🔒"

    /// 暗号化した field(#233)を使う stat か。数える field・並べる field・絞り込み・式(valueExpr)・
    /// それらが computed ならその式の参照先までたどる。一覧の数字は端末に平文で残るので、こうした stat は数えない。
    static func touchesEncrypted(_ c: Component, in dsl: AppDSL) -> Bool {
        let encrypted = dsl.encryptedFields
        guard !encrypted.isEmpty else { return false }
        var seen: Set<String> = []
        func field(_ entity: String, _ id: String?) -> Bool {
            guard let id else { return false }
            if encrypted[entity]?.contains(id) == true { return true }
            guard seen.insert("\(entity).\(id)").inserted, let computed = dsl.entity(entity)?.field(id)?.computed else { return false }
            return expr(computed, row: entity)
        }
        /// `{"ref":"chore.points"}`: ref の field をたどった先の field も見る。
        func refPath(_ entity: String, _ path: String) -> Bool {
            var current = entity
            for id in path.split(separator: ".").map(String.init) {
                if field(current, id) { return true }
                guard let next = dsl.entity(current)?.field(id)?.refEntity else { return false }
                current = next
            }
            return false
        }
        func filters(_ entity: String, _ list: [Filter]) -> Bool {
            list.contains { field(entity, $0.field) || $0.valueExpr.map { expr($0, row: nil) } == true }
        }
        func expr(_ e: Expr, row: String?) -> Bool {
            switch e {
            case .literal: return false
            case .ref(let path): return row.map { refPath($0, path) } ?? false
            case .setting(let id): return dsl.entities.contains { $0.single && field($0.id, id) }
            case .agg(let a): return field(a.entity, a.field) || field(a.entity, a.by) || filters(a.entity, a.filters)
            case .op(_, let args): return args.contains { expr($0, row: row) }
            case .cond(let i, let t, let el): return expr(i, row: row) || expr(t, row: row) || expr(el, row: row)
            }
        }
        if c.aggregate == nil, let v = c.valueExpr { return expr(v, row: nil) }
        guard let e = c.entity else { return false }
        return field(e, c.field) || field(e, c.by) || filters(e, (c.filter.map { [$0] } ?? []) + c.filters)
    }

    /// 1 アプリの数字。stat が無い・出さない・記録が未取得なら nil。暗号化した項目を使う stat は「🔒」(復号できても数えない)。
    @MainActor
    static func compute(dsl: AppDSL, records: [String: [Record]], setting: String?, now: Date = .now) -> Value? {
        compute(Input(dsl: dsl, records: records, setting: setting,
                      countUnit: WidgetSnapshotWriter.countUnit, now: now))
    }

    /// `compute` の main actor の外で呼ぶ版。
    static func compute(_ input: Input) -> Value? {
        guard let c = chosen(in: input.dsl, setting: input.setting) else { return nil }
        let label = c.label ?? ""
        if touchesEncrypted(c, in: input.dsl) { return Value(statId: c.id, label: label, value: locked, at: input.now, statKey: WidgetSnapshotWriter.statKey(c)) }
        let records = input.records
        let evaluator = ExprEvaluator(dsl: input.dsl, records: { records[$0] ?? [] })
        guard let text = WidgetSnapshotWriter.statText(c, dsl: input.dsl, records: records, evaluator: evaluator,
                                                        countUnit: input.countUnit) else { return nil }
        return Value(statId: c.id, label: label, value: text, at: input.now, statKey: WidgetSnapshotWriter.statKey(c))
    }

    /// 重い処理を main actor の外で走らせる。
    static func offMain<T: Sendable>(_ work: @escaping @Sendable () -> T) async -> T {
        await Task.detached(priority: .userInitiated) { work() }.value
    }

    /// カードの 2 行目(「今月 ¥38,200」)。
    static func line(_ v: Value) -> String {
        v.label.isEmpty ? v.value : "\(v.label) \(v.value)"
    }

    /// VoiceOver: 「アプリ名, 数字のラベル, 値」。
    static func accessibilityLabel(appName: String, value: Value?) -> String {
        ([appName] + [value?.label, value?.value].compactMap { $0 }).filter { !$0.isEmpty }.joined(separator: ", ")
    }
}

/// 端末の中だけで数字を作る読み口。記録の API は呼ばない(一覧を開くたびの取得の回数と費用を増やさない)。
/// 後でサーバーが数字を返す形(`GET /me/glance`)に替える時はここを差し替える。
/// `document` と `open` はファイルの読み出し・復号をするので main actor の外で呼ぶ。
struct AppGlanceSource {
    /// 端末に残っている DSL(`DocumentCache`)。無ければ nil。
    var document: @Sendable (String) -> AppDSL?
    /// ローカル DB(`RecordSyncEngine.cachedSealed`)の記録。暗号文のままでよい(`open` で開く)。
    var cached: @MainActor (_ appId: String, _ entity: String) async -> [Record]
    /// 暗号文を開く(AES-GCM)。既定は何もしない。
    var open: @Sendable ([Record]) -> [Record] = { $0 }
    /// 数える処理。
    var compute: AppGlance.Compute = { AppGlance.compute($0) }
}

/// 一覧の数字の置き場(appId → 値・時刻)。端末に保存し、次に一覧を開いた時はすぐ出す。サインアウトで消す。
@MainActor
@Observable
final class AppGlanceStore {
    static let shared = AppGlanceStore()
    static let defaultsKey = "metamon.appGlance.v2"
    /// 前の形式(暗号化した項目の値も残していた)。読まずに消す
    static let legacyDefaultsKeys = ["metamon.appGlance.v1"]
    /// 同じアプリを読み直すまでの間(一覧を出し入れするたびにローカル DB を読まない)。
    static let recheckAfter: TimeInterval = 60

    private(set) var values: [String: AppGlance.Value] = [:]
    /// 長押しメニューの選択肢(appId → stat の id と名前)。
    private(set) var choices: [String: [(id: String, label: String)]] = [:]
    /// 読み込み中(値がまだ無いカードはスケルトン)。
    private(set) var loading: Set<String> = []
    @ObservationIgnored private var checkedAt: [String: Date] = [:]
    /// 読み込み中(DSL の読み出しを含む・同じアプリを重ねて読まない)。
    @ObservationIgnored private var inFlight: Set<String> = []
    /// `clear()` で増やす。読み込み中に消された時、前の利用者の値を書き戻さない
    @ObservationIgnored private var generation = 0
    @ObservationIgnored private let defaults: UserDefaults

    init(defaults: UserDefaults = .standard) {
        self.defaults = defaults
        Self.legacyDefaultsKeys.forEach { defaults.removeObject(forKey: $0) }
        if let data = defaults.data(forKey: Self.defaultsKey),
           let saved = try? JSONDecoder().decode([String: AppGlance.Value].self, from: data) {
            values = saved
        }
        #if DEBUG
        if ProcessInfo.processInfo.environment["METAMON_MOCK_GLANCE"] == "1" { // スクリーンショット用(METAMON_MOCK_SHARING の 2 件に数字)
            seed(["uitest-kakeibo": .init(statId: "month_total", label: "今月", value: "¥38,200", at: .now),
                  "uitest-shopping": .init(statId: "open_count", label: "未購入", value: "4件", at: .now.addingTimeInterval(-3 * 60 * 60))])
        }
        #endif
    }

    /// 見えているカード 1 枚分を端末の中の DSL・記録で作り直す。DSL が端末に無いアプリは前の値のまま。
    /// DSL の読み出し・復号・集計は main actor の外。main では入力の写しと結果の反映だけをする。
    func load(appId: String, setting: String?, source: AppGlanceSource, now: Date = .now) async {
        if let at = checkedAt[appId], now.timeIntervalSince(at) < Self.recheckAfter { return }
        guard !inFlight.contains(appId) else { return }
        checkedAt[appId] = now
        inFlight.insert(appId)
        defer { inFlight.remove(appId) }
        let started = generation
        let read = source.document
        guard let dsl = await AppGlance.offMain({ read(appId) }), started == generation else { return }
        loading.insert(appId)
        defer { loading.remove(appId) }
        choices[appId] = AppGlance.choices(in: dsl).map { ($0.id, $0.label ?? "") }
        guard let c = AppGlance.chosen(in: dsl, setting: setting) else {
            put(appId, nil)
            return
        }
        if AppGlance.touchesEncrypted(c, in: dsl) { // 記録は読まない
            put(appId, AppGlance.compute(.init(dsl: dsl, records: [:], setting: setting, countUnit: WidgetSnapshotWriter.countUnit, now: now)))
            return
        }
        var sealed: [String: [Record]] = [:]
        for e in AppGlance.entities(for: c, in: dsl) {
            sealed[e] = await source.cached(appId, e)
        }
        guard started == generation else { return } // 読んでいる間にサインアウトした
        // この端末でまだ開いていないアプリは記録が空。「0 件」と言い切らず、前の値のまま(無ければ出さない)
        guard sealed.values.contains(where: { !$0.isEmpty }) || c.aggregate == nil && c.valueExpr != nil else { return }
        let open = source.open, compute = source.compute, countUnit = WidgetSnapshotWriter.countUnit, rows = sealed
        let value = await AppGlance.offMain {
            compute(.init(dsl: dsl, records: rows.mapValues(open), setting: setting, countUnit: countUnit, now: now))
        }
        guard started == generation else { return } // 数えている間にサインアウトした
        put(appId, value)
    }

    /// 使用中のアプリは画面に読み込んだ記録(復号済み)で作る。集計は main actor の外。
    func update(appId: String, dsl: AppDSL, records: [String: [Record]], setting: String?, now: Date = .now,
                compute: @escaping AppGlance.Compute = { AppGlance.compute($0) }) async {
        choices[appId] = AppGlance.choices(in: dsl).map { ($0.id, $0.label ?? "") }
        guard let c = AppGlance.chosen(in: dsl, setting: setting) else {
            put(appId, nil)
            return
        }
        // 記録がまだ届いていない間は前の値を残す
        guard AppGlance.entities(for: c, in: dsl).allSatisfy({ records[$0] != nil }) else { return }
        let started = generation
        let input = AppGlance.Input(dsl: dsl, records: records, setting: setting,
                                    countUnit: WidgetSnapshotWriter.countUnit, now: now)
        let value = await AppGlance.offMain { compute(input) }
        guard started == generation else { return }
        put(appId, value)
    }

    /// 選び直した時: 次の `load` で必ず読み直す。
    func invalidate(_ appId: String) {
        checkedAt[appId] = nil
    }

    func forget(_ appId: String) {
        checkedAt[appId] = nil
        choices[appId] = nil
        put(appId, nil)
    }

    /// サインアウト・アカウント削除(前の人の数字を残さない)。
    func clear() {
        generation += 1
        values = [:]
        choices = [:]
        loading = []
        checkedAt = [:]
        defaults.removeObject(forKey: Self.defaultsKey)
    }

    #if DEBUG
    /// スクリーンショット用(`METAMON_MOCK_GLANCE=1`)。
    func seed(_ seeded: [String: AppGlance.Value], now: Date = .now) {
        for (id, v) in seeded {
            values[id] = v
            checkedAt[id] = now.addingTimeInterval(24 * 60 * 60)
        }
    }
    #endif

    private func put(_ appId: String, _ value: AppGlance.Value?) {
        guard values[appId] != value else { return }
        values[appId] = value
        // 「🔒」は端末に残さない(次に開いた時は読み直す)
        if let data = try? JSONEncoder().encode(values.filter { $0.value.value != AppGlance.locked }) {
            defaults.set(data, forKey: Self.defaultsKey)
        }
    }
}
