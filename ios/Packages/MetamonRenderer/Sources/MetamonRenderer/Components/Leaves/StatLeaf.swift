import SwiftUI
import MetamonDSL

/// `stat`: entity の集計値(count|sum|avg|min|max)を1枚のカードに。
/// 見た目: 小さなブランド色アイコン + ラベル(secondary)、数値は rounded bold 等幅。カードの左上に極薄のブランドグラデーション。
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
        VStack(alignment: .leading, spacing: RenderTheme.Spacing.sm) {
            HStack(spacing: 6) {
                Image(systemName: c.icon ?? FieldIcon.stat(aggregate: c.aggregate, field: fieldDef))
                    .font(.caption.weight(.bold))
                    .foregroundStyle(RenderTheme.Colors.brandA)
                    .frame(width: 20, height: 20)
                    .background(RenderTheme.Colors.brandSoft, in: RoundedRectangle(cornerRadius: 6, style: .continuous))
                    .accessibilityHidden(true)
                Text(c.label ?? "")
                    .font(RenderTheme.Fonts.statLabel)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
                    .minimumScaleFactor(0.8)
            }
            Text(Aggregator.format(value, as: format, unit: c.aggregate == "count" ? "件" : unit))
                .font(RenderTheme.Fonts.statValue)
                .lineLimit(1)
                .minimumScaleFactor(0.6)
                .contentTransition(.numericText())
        }
        .renderCard(wash: true)
    }
}
