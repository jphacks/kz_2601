import SwiftUI
import MetamonDSL

/// `stat`: entity の集計値(count|sum|avg|min|max)を1枚のカードに。数値は大きく・等幅数字、ラベルは secondary。
/// VoiceOver はカード 1 要素として「今月の支出, 1,400円」と読む。Dynamic Type(アクセシビリティサイズ)ではラベルを 2 行まで許す。
struct StatLeaf: View {
    @Environment(\.dslRuntime) private var rt
    @Environment(\.dynamicTypeSize) private var typeSize
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
        let formatted = Aggregator.format(value, as: format, unit: c.aggregate == "count" ? "件" : unit)
        // アクセシビリティサイズ: 1 行高のセルでも数値が見えるよう、ラベルは縮小 1 行・余白を詰める(足りなければセル内スクロール)
        let compact = typeSize.isAccessibilitySize
        VStack(alignment: .leading, spacing: RenderTheme.Spacing.xs) {
            Text(c.label ?? "")
                .font(RenderTheme.Fonts.statLabel)
                .foregroundStyle(.secondary)
                .lineLimit(1)
                .minimumScaleFactor(compact ? 0.6 : 0.8)
            Text(formatted)
                .font(RenderTheme.Fonts.statValue)
                .lineLimit(1)
                .minimumScaleFactor(0.6)
                .contentTransition(.numericText())
        }
        .renderCard(padding: compact ? RenderTheme.Spacing.sm : RenderTheme.Spacing.md)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(c.label ?? "")
        .accessibilityValue(formatted)
    }
}
