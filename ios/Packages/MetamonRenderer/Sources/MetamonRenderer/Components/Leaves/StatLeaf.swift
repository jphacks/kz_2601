import SwiftUI
import MetamonDSL

/// `stat`: entity の集計値(count|sum|avg|min|max)を1枚のカードに。数値は大きく・等幅数字、ラベルは secondary。
struct StatLeaf: View {
    @Environment(\.dslRuntime) private var rt
    let c: Component

    var body: some View {
        let entityId = c.entity ?? ""
        let entity = rt.dsl.entity(entityId)
        let rows = Aggregator.apply(c.filter, entity: entity, to: rt.records(for: entityId))
        let value = Aggregator.aggregate(c.aggregate, field: c.field, over: rows)
        let fieldDef = c.field.flatMap { entity?.field($0) }
        let unit = fieldDef?.unit
        // format 省略時: count → 件数、currency 型の field → ¥ と桁区切り
        let format = c.format ?? (c.aggregate == "count" ? "plain" : (fieldDef?.type == .currency ? "currency" : nil))
        // 取得失敗(#85)は「¥0」ではなく「—」(データ無しと区別)。ローカルに何かあれば通常どおり集計する
        let unavailable = rows.isEmpty && rt.emptyState(for: entityId) == .unavailable
        // 初回ロード中は 0 を見せず placeholder(起動直後に「¥0 → ¥480」と跳ねるのを防ぐ)
        let isLoading = rt.loading.contains(entityId) && rt.records(for: entityId).isEmpty
        VStack(alignment: .leading, spacing: RenderTheme.Spacing.xs) {
            Text(c.label ?? "")
                .font(RenderTheme.Fonts.statLabel)
                .foregroundStyle(.secondary)
                .lineLimit(1)
            // 大きめの基準サイズから、セルの高さに収まるまで縮める(「もっと大きく」で area を広げると数字も大きくなる)
            Text(isLoading ? "0000" : unavailable ? EmptyPlaceholder.unavailableValue : Aggregator.format(value, as: format, unit: c.aggregate == "count" ? "件" : unit))
                .font(RenderTheme.Fonts.statValue)
                .lineLimit(1)
                .minimumScaleFactor(0.45)
                .contentTransition(.numericText())
                .redacted(reason: isLoading ? .placeholder : [])
        }
        // セル全体をカードにする(area を縦に広げた時にカードの下だけ空くのを防ぐ)
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        .renderCard()
    }
}
