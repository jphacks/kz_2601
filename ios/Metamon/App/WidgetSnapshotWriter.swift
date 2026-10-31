import Foundation
import WidgetKit
import MetamonDSL

/// DSL の `stat` 部品(先頭 3 個)を本体側で集計し、App Group 経由でウィジェットに渡す。
/// systemLarge(#486)用に、先頭 stat の entity の「最近の記録」と「直近 7 日」の日ごとの集計も入れる。
/// 呼び出し元は `AppRuntime.apply()`(DSL 差し替え)と `AppRuntime.refresh()`(レコード更新)の 2 箇所。
/// 集計ロジックは Renderer の `StatLeaf` と同じ(`Aggregator`)なので、アプリ内の表示と数字が一致する。
@MainActor
enum WidgetSnapshotWriter {
    static let maxStats = 3
    /// systemLarge の「最近の記録」の最大行数(#486)。
    static let maxRecent = 5
    static let trendDays = 7

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
        let snapshot = makeSnapshot(dsl: dsl, records: records, previous: previous)
        // 中身が変わっていなければ書き込み・reload しない(WidgetKit の更新回数予算を節約)
        guard !snapshot.hasSameContent(as: previous) else { return }
        snapshot.save()
        WidgetCenter.shared.reloadAllTimelines()
    }

    /// スナップショットを組み立てる(保存はしない・テスト用に分けてある)。
    static func makeSnapshot(dsl: AppDSL, records: [String: [Record]], previous: WidgetSnapshot?,
                             now: Date = .now, calendar: Calendar = .current) -> WidgetSnapshot {
        // computed field / Stat.value(#101)も StatLeaf と同じ評価器で数える
        let evaluator = ExprEvaluator(dsl: dsl, records: { records[$0] ?? [] })
        let components = statComponents(in: dsl)
        let stats = components.prefix(maxStats).map { c -> WidgetSnapshot.Stat in
            let entityId = c.entity ?? ""
            let key = [entityId, c.aggregate ?? "count", c.field ?? "", c.label ?? ""].joined(separator: "|")
            // レコード未取得(apply 直後など)は前回値を引き継ぎ、ウィジェットが一瞬 0 にならないようにする
            let value = statText(c, dsl: dsl, records: records, evaluator: evaluator) ?? previous?.stats.first { $0.key == key }?.value
            return WidgetSnapshot.Stat(key: key, label: c.label ?? "", value: value)
        }
        var snapshot = WidgetSnapshot(appName: dsl.app.name, stats: Array(stats), updatedAt: now)

        // systemLarge(#486): 先頭の(entity を数える)stat の entity で「最近の記録」と「直近 7 日」を作る
        guard let primary = components.prefix(maxStats).first(where: { $0.entity.flatMap(dsl.entity) != nil && !($0.aggregate == nil && $0.valueExpr != nil) }),
              let entity = primary.entity.flatMap(dsl.entity) else { return snapshot }
        snapshot.recentTitle = entity.name
        guard records[entity.id] != nil else {
            // 未取得は前回分を引き継ぐ(同じ entity の時だけ)
            if previous?.recentTitle == entity.name {
                snapshot.recent = previous?.recent
                snapshot.trend = previous?.trend
            }
            return snapshot
        }
        let rows = evaluator.viewRecords(for: entity.id)
        snapshot.recent = recentRows(entity: entity, records: rows, preferredValueField: primary.field, now: now)
        snapshot.trend = trend(primary, entity: entity, records: rows, now: now, calendar: calendar)
        return snapshot
    }

    /// stat 1 個の表示文字列。レコード未取得なら nil(ライブアクティビティの数字 #423 と共用)。
    static func statText(_ c: Component, dsl: AppDSL, records: [String: [Record]], evaluator: ExprEvaluator) -> String? {
        if c.aggregate == nil, let expr = c.valueExpr {
            return evaluator.evaluate(expr).map { v in
                if case .number(let n) = v { return Aggregator.format(n, as: c.format, unit: c.unit) }
                return v.text
            } ?? "—"
        }
        let entityId = c.entity ?? ""
        guard records[entityId] != nil else { return nil }
        return formatted(c, entity: dsl.entity(entityId), records: evaluator.viewRecords(for: entityId))
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
        return formattedValue(c, entity: entity, value: Aggregator.aggregate(c.aggregate, field: c.field, over: rows))
    }

    /// 集計値 → stat と同じ書式の文字列。
    static func formattedValue(_ c: Component, entity: Entity?, value: Double) -> String {
        let unit = c.field.flatMap { entity?.field($0)?.unit }
        return Aggregator.format(
            value,
            as: c.format ?? (c.aggregate == "count" ? "plain" : nil),
            unit: c.aggregate == "count" ? L10n.tr("件") : unit
        )
    }

    // MARK: - systemLarge(#486)

    /// 新しい順(作成日時)に `maxRecent` 件。hidden の field(#262)は出さない。
    static func recentRows(entity: Entity, records: [Record], preferredValueField: String?, now: Date = .now) -> [WidgetSnapshot.Row] {
        let visible = entity.fields.filter { !$0.hidden }
        let titleField = entity.displayTitleField.flatMap(entity.field)
        let valueField = preferredValueField.flatMap(entity.field).flatMap { $0.hidden || !$0.type.isNumeric ? nil : $0 }
            ?? visible.first { $0.type.isNumeric }
        let dateField = visible.first { $0.type.isDate }
        let rows = HiddenFields.redact(Aggregator.sorted(records, by: nil, entity: entity), in: entity).prefix(maxRecent)
        return rows.map { r in
            let title = titleField.map { FieldCoding.display(r[$0.id], field: $0, now: now) } ?? ""
            let detail: String? = if let dateField, !r[dateField.id].isNull {
                FieldCoding.display(r[dateField.id], field: dateField, now: now)
            } else {
                ChatThread.date(r.createdAt).map { FieldCoding.displayDate($0, withTime: false, locale: .current, now: now) }
            }
            let value = valueField.flatMap { r[$0.id].isNull ? nil : FieldCoding.display(r[$0.id], field: $0, now: now) }
            return WidgetSnapshot.Row(id: r.id, title: title.isEmpty ? FieldCoding.unavailable : title,
                                      detail: detail, value: value)
        }
    }

    /// 先頭 stat の集計を日ごとに `trendDays` 日ぶん(今日まで)。latest / earliest(#133)は日ごとにできないので nil。
    /// 日付は entity の最初の date field(無ければ作成日時)。stat の期間 filter(今月など)は外し、それ以外の filter は掛ける。
    static func trend(_ c: Component, entity: Entity, records: [Record], now: Date = .now, calendar: Calendar = .current) -> WidgetSnapshot.Trend? {
        guard !Aggregator.isPick(c.aggregate) else { return nil }
        let periodOps: Set<String> = ["today", "thisWeek", "thisMonth", "thisYear"]
        let filtered = c.filter.map { periodOps.contains($0.op) ? records : Aggregator.apply($0, entity: entity, to: records) } ?? records
        let dateField = entity.fields.first { !$0.hidden && $0.type.isDate }
        let today = calendar.startOfDay(for: now)
        let days = (0..<trendDays).compactMap { calendar.date(byAdding: .day, value: $0 - (trendDays - 1), to: today) }
        var byDay: [Date: [Record]] = [:]
        for r in filtered {
            let d = dateField.flatMap { FieldCoding.date(from: r[$0.id]) } ?? ChatThread.date(r.createdAt)
            guard let d else { continue }
            let day = calendar.startOfDay(for: d)
            guard day >= days[0], day <= today else { continue }
            byDay[day, default: []].append(r)
        }
        let points = days.map { WidgetSnapshot.Trend.Point(day: $0, value: Aggregator.aggregate(c.aggregate, field: c.field, over: byDay[$0] ?? [])) }
        let inRange = days.flatMap { byDay[$0] ?? [] }
        let total = formattedValue(c, entity: entity, value: Aggregator.aggregate(c.aggregate, field: c.field, over: inRange))
        let metric = c.aggregate == nil || c.aggregate == "count" ? nil : c.field.flatMap { entity.field($0)?.name }
        return WidgetSnapshot.Trend(metric: metric, total: total, points: points)
    }
}
