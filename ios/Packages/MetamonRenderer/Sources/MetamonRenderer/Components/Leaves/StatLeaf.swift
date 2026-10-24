import SwiftUI
import MetamonDSL

/// `stat`: entity の集計値(count|sum|avg|min|max)を1枚のカードに。
struct StatLeaf: View {
    @Environment(\.dslRuntime) private var rt
    let c: Component

    var body: some View {
        let entityId = c.entity ?? ""
        let entity = rt.dsl.entity(entityId)
        let rows = Aggregator.apply(c.filter, entity: entity, to: rt.records(for: entityId))
        let value = Aggregator.aggregate(c.aggregate, field: c.field, over: rows)
        let unit = c.field.flatMap { entity?.field($0)?.unit }
        VStack(alignment: .leading, spacing: 4) {
            Text(c.label ?? "").font(.caption).foregroundStyle(.secondary)
            Text(Aggregator.format(value, as: c.format ?? (c.aggregate == "count" ? "plain" : nil), unit: c.aggregate == "count" ? "件" : unit))
                .font(.title2.bold().monospacedDigit())
                .contentTransition(.numericText())
        }
        .padding(16)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(Color(.secondarySystemGroupedBackground), in: RoundedRectangle(cornerRadius: 16))
    }
}
