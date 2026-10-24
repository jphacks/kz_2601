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
                    Text(label).font(.caption).foregroundStyle(.secondary)
                    Spacer()
                    Text(String(format: "%.0f%%", ratio * 100)).font(.caption.monospacedDigit()).foregroundStyle(.secondary)
                }
            }
            ProgressView(value: ratio)
                .tint(ratio >= 1 ? Color.semantic("red") : .accentColor)
                .animation(reduceMotion ? nil : .easeOut(duration: 0.3), value: ratio)
        }
        .renderCard()
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(c.label ?? "進捗")
        .accessibilityValue(String(format: "%.0f%%", ratio * 100))
    }

    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    private var resolvedRatio: Double {
        if let v = c.value { return v }
        guard let entityId = c.entity, let target = c.target, target > 0 else { return 0 }
        let rows = rt.filteredRecords(c)
        return Aggregator.aggregate(c.aggregate ?? "sum", field: c.field, over: rows) / target
    }
}
