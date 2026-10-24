import SwiftUI
import MetamonDSL

/// `progressBar`: 目標に対する進捗バー(ProgressView)。`value`(0..1)リテラル、または entity 集計 `aggregate(field) / target`。
/// 集計は `stat` と同じ経路(`DSLRuntime.records` + `Aggregator`)。
struct ProgressBarLeaf: View {
    @Environment(\.dslRuntime) private var rt
    let c: Component

    var body: some View {
        let ratio = max(0, min(1, resolvedRatio))
        VStack(alignment: .leading, spacing: 6) {
            if let label = c.label {
                HStack {
                    Text(label).font(RenderTheme.Fonts.statLabel).foregroundStyle(.secondary)
                    Spacer()
                    Text(String(format: "%.0f%%", ratio * 100))
                        .font(.system(.subheadline, design: .rounded).weight(.semibold).monospacedDigit())
                        .foregroundStyle(ratio >= 1 ? RenderTheme.Colors.warning : RenderTheme.Colors.brandA)
                }
            }
            GeometryReader { g in
                ZStack(alignment: .leading) {
                    Capsule().fill(RenderTheme.Colors.surfaceElevated)
                    Capsule()
                        .fill(ratio >= 1 ? AnyShapeStyle(RenderTheme.Colors.warning) : AnyShapeStyle(RenderTheme.brandGradient))
                        .frame(width: g.size.width * ratio)
                }
            }
            .frame(height: 10)
            .animation(.easeOut(duration: 0.3), value: ratio)
            .accessibilityElement(children: .ignore)
            .accessibilityLabel(c.label ?? "")
            .accessibilityValue(String(format: "%.0f%%", ratio * 100))
        }
        .renderCard()
    }

    private var resolvedRatio: Double {
        if let v = c.value { return v }
        guard let entityId = c.entity, let target = c.target, target > 0 else { return 0 }
        let entity = rt.dsl.entity(entityId)
        let rows = Aggregator.apply(c.filter, entity: entity, to: rt.records(for: entityId))
        return Aggregator.aggregate(c.aggregate ?? "sum", field: c.field, over: rows) / target
    }
}
