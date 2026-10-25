import Foundation
import WidgetKit
import MetamonDSL

/// DSL の `stat` 部品(先頭 3 個)を本体側で集計し、App Group 経由でウィジェットに渡す。
/// 呼び出し元は `AppRuntime.apply()`(DSL 差し替え)と `AppRuntime.refresh()`(レコード更新)の 2 箇所。
/// 集計ロジックは Renderer の `StatLeaf` と同じ(`Aggregator`)なので、アプリ内の表示と数字が一致する。
@MainActor
enum WidgetSnapshotWriter {
    static let maxStats = 3

    static func write(dsl: AppDSL, records: [String: [Record]]) {
        let previous = WidgetSnapshot.load()
        // 初期状態(音声画面のみ)はスナップショットを消して「アプリを作るとここに出ます」表示に戻す
        guard !dsl.isVoiceOnly else {
            if previous != nil {
                WidgetSnapshot.clear()
                WidgetCenter.shared.reloadAllTimelines()
            }
            return
        }
        // computed field / Stat.value(#101)も StatLeaf と同じ評価器で数える
        let evaluator = ExprEvaluator(dsl: dsl, records: { records[$0] ?? [] })
        let stats = statComponents(in: dsl).prefix(maxStats).map { c -> WidgetSnapshot.Stat in
            let entityId = c.entity ?? ""
            let key = [entityId, c.aggregate ?? "count", c.field ?? "", c.label ?? ""].joined(separator: "|")
            let value: String?
            if c.aggregate == nil, let expr = c.valueExpr {
                value = evaluator.evaluate(expr).map { v in
                    if case .number(let n) = v { return Aggregator.format(n, as: c.format, unit: c.unit) }
                    return v.text
                } ?? "—"
            } else if records[entityId] != nil {
                value = formatted(c, entity: dsl.entity(entityId), records: evaluator.viewRecords(for: entityId))
            } else {
                // レコード未取得(apply 直後など)は前回値を引き継ぎ、ウィジェットが一瞬 0 にならないようにする
                value = previous?.stats.first { $0.key == key }?.value
            }
            return WidgetSnapshot.Stat(key: key, label: c.label ?? "", value: value)
        }
        let snapshot = WidgetSnapshot(appName: dsl.app.name, stats: Array(stats), updatedAt: .now)
        // 値もラベルも変わっていなければ書き込み・reload しない(WidgetKit の更新回数予算を節約)
        guard snapshot.stats != previous?.stats || snapshot.appName != previous?.appName else { return }
        snapshot.save()
        WidgetCenter.shared.reloadAllTimelines()
    }

    /// 画面順(initialScreen → タブ順 → 残り)で `stat` を列挙する。
    static func statComponents(in dsl: AppDSL) -> [Component] {
        var order = [dsl.initialScreen] + dsl.app.tabs.map(\.screen)
        order += dsl.screens.map(\.id).filter { !order.contains($0) }
        var seen = Set<String>()
        return order
            .filter { seen.insert($0).inserted }
            .compactMap { dsl.screen($0) }
            .flatMap(\.allComponents)
            .filter { $0.type == .stat }
    }

    /// `StatLeaf` と同じ式で表示文字列にする。
    static func formatted(_ c: Component, entity: Entity?, records: [Record]) -> String {
        let rows = Aggregator.apply(c.filter, entity: entity, to: records) // filter.value の Expr は未対応(ウィジェットは行コンテキスト無し)
        if Aggregator.isPick(c.aggregate) { return Aggregator.pickText(c, entity: entity, records: rows) ?? "—" } // latest / earliest(#133)
        let value = Aggregator.aggregate(c.aggregate, field: c.field, over: rows)
        let unit = c.field.flatMap { entity?.field($0)?.unit }
        return Aggregator.format(
            value,
            as: c.format ?? (c.aggregate == "count" ? "plain" : nil),
            unit: c.aggregate == "count" ? L10n.tr("件") : unit
        )
    }
}
