import Foundation
import Observation
import MetamonDSL
import MetamonServices

/// メニューバーに出す stat 1 個(表示文字列は整形済み)
struct MacMenuBarStat: Equatable, Identifiable {
    let id: String
    let label: String
    let value: String
}

/// 生成アプリの先頭の `stat` を集計する(#1848)。並びと式はウィジェットの `WidgetSnapshotWriter` をそのまま使うので(#1969)、
/// アプリ内の stat・ウィジェットと数字がそろう
@MainActor
enum MacMenuBarStats {
    static var maxStats: Int { WidgetSnapshotWriter.maxStats }

    /// 先頭 `maxStats` 個の表示。記録をまだ読めていない stat は「—」
    static func make(dsl: AppDSL, records: [String: [Record]]) -> [MacMenuBarStat] {
        let evaluator = ExprEvaluator(dsl: dsl, records: { records[$0] ?? [] })
        return WidgetSnapshotWriter.statComponents(in: dsl).prefix(maxStats).enumerated().map { index, c in
            MacMenuBarStat(id: "\(index)|\(c.entity ?? "")|\(c.aggregate ?? "count")|\(c.field ?? "")",
                           label: c.label ?? "",
                           value: WidgetSnapshotWriter.statText(c, dsl: dsl, records: records, evaluator: evaluator) ?? "—")
        }
    }

    /// 集計に要る entity(記録を読むもの)
    static func entities(in dsl: AppDSL) -> [String] {
        var seen = Set<String>()
        return WidgetSnapshotWriter.statComponents(in: dsl).prefix(maxStats).compactMap(\.entity).filter { seen.insert($0).inserted }
    }
}

// MARK: - 読み込み元

/// メニューバーが読むもの(テストでは偽物に替える)。本物は `IntentHost`(ショートカットと同じ一覧・同じ記録のエンジン)
@MainActor
protocol MacMenuBarSource {
    func menuBarApps() async -> [AppSummary]
    func menuBarDocument(appId: String) async -> AppDocument?
    func menuBarRecords(appId: String, entity: String) async -> [Record]
}

extension IntentHost: MacMenuBarSource {
    /// 記録が端末に残る作ったアプリだけ(同梱の例の記録はウインドウを閉じると消える)
    func menuBarApps() async -> [AppSummary] { await recordableApps() }
    func menuBarDocument(appId: String) async -> AppDocument? { await document(appId: appId) }
    func menuBarRecords(appId: String, entity: String) async -> [Record] { await cachedRecords(appId: appId, entity: entity) }
}

/// メニューバーの窓の中身: アプリの一覧・選んだアプリ・その stat
@MainActor
@Observable
final class MacMenuBarModel {
    private(set) var apps: [AppSummary] = []
    private(set) var selectedId: String?
    private(set) var stats: [MacMenuBarStat] = []
    private(set) var isLoading = false

    @ObservationIgnored private let source: any MacMenuBarSource
    @ObservationIgnored private let settings: MacMenuBarSettings
    @ObservationIgnored private var generation = 0

    init(source: (any MacMenuBarSource)? = nil, settings: MacMenuBarSettings = MacMenuBarSettings()) {
        self.source = source ?? IntentHost.shared
        self.settings = settings
    }

    var selectedApp: AppSummary? { apps.first { $0.id == selectedId } }

    /// 一覧を引き直し、選んでいたアプリ(無くなっていれば先頭)の stat を出す
    func reload() async {
        apps = await source.menuBarApps()
        let saved = settings.selectedAppId
        selectedId = apps.contains { $0.id == saved } ? saved : apps.first?.id
        await loadStats()
    }

    /// 選ぶ(次に開いた時も同じアプリ)
    func select(_ appId: String) async {
        guard apps.contains(where: { $0.id == appId }) else { return }
        selectedId = appId
        settings.selectedAppId = appId
        await loadStats()
    }

    /// 記録が増えた(`IntentHost.recordAdded`)。選んでいるアプリの時だけ数え直す
    func recordsChanged(appId: String?) async {
        guard appId == nil || appId == selectedId else { return }
        await loadStats()
    }

    private func loadStats() async {
        generation += 1
        let current = generation
        guard let appId = selectedId else { stats = []; return }
        isLoading = true
        defer { if current == generation { isLoading = false } }
        guard let dsl = await source.menuBarDocument(appId: appId)?.dsl else {
            if current == generation { stats = [] }
            return
        }
        var records: [String: [Record]] = [:]
        for entity in MacMenuBarStats.entities(in: dsl) {
            records[entity] = await source.menuBarRecords(appId: appId, entity: entity)
        }
        guard current == generation else { return } // 途中で別のアプリを選んだ
        stats = MacMenuBarStats.make(dsl: dsl, records: records)
    }
}
