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
    }

    /// 数えるのに要るものの写し。言語の設定(件数の単位)は main actor で引いて入れる。
    struct Input: Sendable {
        var dsl: AppDSL
        var records: [String: [Record]]
        var setting: String?
        var canDecrypt: Bool
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

    /// 暗号化した field(#233)を数える stat。
    static func isEncrypted(_ c: Component, in dsl: AppDSL) -> Bool {
        guard let e = c.entity, let f = c.field else { return false }
        return dsl.encryptedFields[e]?.contains(f) ?? false
    }

    /// 1 アプリの数字。stat が無い・出さない・記録が未取得なら nil。鍵の無い暗号化 field は「🔒」。
    @MainActor
    static func compute(dsl: AppDSL, records: [String: [Record]], setting: String?, canDecrypt: Bool = true, now: Date = .now) -> Value? {
        compute(Input(dsl: dsl, records: records, setting: setting, canDecrypt: canDecrypt,
                      countUnit: WidgetSnapshotWriter.countUnit, now: now))
    }

    /// `compute` の main actor の外で呼ぶ版。
    static func compute(_ input: Input) -> Value? {
        guard let c = chosen(in: input.dsl, setting: input.setting) else { return nil }
        let label = c.label ?? ""
        if !input.canDecrypt, isEncrypted(c, in: input.dsl) { return Value(statId: c.id, label: label, value: "🔒", at: input.now) }
        let records = input.records
        let evaluator = ExprEvaluator(dsl: input.dsl, records: { records[$0] ?? [] })
        guard let text = WidgetSnapshotWriter.statText(c, dsl: input.dsl, records: records, evaluator: evaluator,
                                                        countUnit: input.countUnit) else { return nil }
        return Value(statId: c.id, label: label, value: text, at: input.now)
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
    static let defaultsKey = "metamon.appGlance.v1"
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
        var sealed: [String: [Record]] = [:]
        for e in AppGlance.entities(for: c, in: dsl) {
            sealed[e] = await source.cached(appId, e)
        }
        // この端末でまだ開いていないアプリは記録が空。「0 件」と言い切らず、前の値のまま(無ければ出さない)
        guard sealed.values.contains(where: { !$0.isEmpty }) || c.aggregate == nil && c.valueExpr != nil else { return }
        // ローカル DB の記録は暗号化 field を開けないので、その stat は「🔒」
        let open = source.open, compute = source.compute, countUnit = WidgetSnapshotWriter.countUnit, rows = sealed
        let value = await AppGlance.offMain {
            compute(.init(dsl: dsl, records: rows.mapValues(open), setting: setting, canDecrypt: false, countUnit: countUnit, now: now))
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
        let input = AppGlance.Input(dsl: dsl, records: records, setting: setting, canDecrypt: true,
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
        if let data = try? JSONEncoder().encode(values) {
            defaults.set(data, forKey: Self.defaultsKey)
        }
    }
}
