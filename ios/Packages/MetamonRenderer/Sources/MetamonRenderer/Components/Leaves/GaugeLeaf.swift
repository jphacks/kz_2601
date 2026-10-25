import SwiftUI
import MetamonDSL

/// `gauge`: [min,max] の中の値を円形ゲージで(SwiftUI Gauge)。`value` リテラル、または entity 集計(stat と同じ経路)。
struct GaugeLeaf: View {
    @Environment(\.dslRuntime) private var rt
    let c: Component

    var body: some View {
        let lo = c.min ?? 0
        let hi = (c.max ?? 1) > lo ? (c.max ?? 1) : lo + 1
        let (value, unit) = resolvedValue
        let clamped = max(lo, min(hi, value))
        VStack(spacing: 8) {
            Gauge(value: clamped, in: lo...hi) {
                Text(c.label ?? "")
            } currentValueLabel: {
                Text(Aggregator.format(value, as: nil, unit: nil)).font(.caption.monospacedDigit())
            } minimumValueLabel: {
                Text(compact(lo)).font(.caption2)
            } maximumValueLabel: {
                Text(compact(hi)).font(.caption2)
            }
            .gaugeStyle(.accessoryCircular)
            .scaleEffect(1.4)
            .frame(height: 80)
            .tint(Gradient(colors: [RenderTheme.Colors.brandA, RenderTheme.Colors.brandB]))
            if let label = c.label {
                Text(unit.map { "\(label) (\($0))" } ?? label).font(.caption).foregroundStyle(.secondary).lineLimit(1)
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .renderCard()
        // VoiceOver: 「体重 (kg), 62.5(範囲 40〜120)」と 1 要素で読む
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(unit.flatMap { u in c.label.map { "\($0) (\(u))" } } ?? c.label ?? L("ゲージ"))
        .accessibilityValue(L("\(Aggregator.format(value, as: nil, unit: nil))(範囲 \(compact(lo))〜\(compact(hi)))"))
    }

    /// 端の目盛り: 大きい値は 30K のように短く(円形ゲージの幅に収める)。
    private func compact(_ v: Double) -> String {
        abs(v) >= 10_000 ? v.formatted(.number.notation(.compactName).precision(.fractionLength(0...1))) : Aggregator.format(v, as: nil)
    }

    /// (値, 単位)。entity モードは field の unit を単位にする(count は 件)。
    private var resolvedValue: (Double, String?) {
        if let v = c.value { return (v, nil) }
        guard let entityId = c.entity else { return (0, nil) }
        let entity = rt.dsl.entity(entityId)
        let rows = rt.filteredRecords(c)
        let op = c.aggregate ?? "sum"
        let unit = op == "count" ? L("件") : c.field.flatMap { entity?.field($0)?.unit }
        return (Aggregator.aggregate(op, field: c.field, over: rows), unit)
    }
}
