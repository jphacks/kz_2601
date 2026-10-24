import SwiftUI
import MetamonDSL

/// `progressBar`: 目標に対する進捗バー(ProgressView)。`value`(0..1)リテラル、または entity 集計 `aggregate(field) / target`。
/// 集計は `stat` と同じ経路(`DSLRuntime.records` + `Aggregator`)。
struct ProgressBarLeaf: View {
    @Environment(\.dslRuntime) private var rt
    let c: Component

    var body: some View {
        let raw = resolvedRatio
        let ratio = max(0, min(1, raw))
        let style = Self.style(raw: raw, tone: c.tone)
        let percent = String(format: "%.0f%%", (c.tone == nil ? ratio : max(0, raw)) * 100)
        VStack(alignment: .leading, spacing: 6) {
            if let label = c.label {
                HStack {
                    Text(label).font(RenderTheme.Fonts.statLabel).foregroundStyle(.secondary)
                    Spacer()
                    Text(percent)
                        .font(.system(.subheadline, design: .rounded).weight(.semibold).monospacedDigit())
                        .foregroundStyle(style.text)
                }
            }
            GeometryReader { g in
                ZStack(alignment: .leading) {
                    Capsule().fill(RenderTheme.Colors.surfaceElevated)
                    Capsule()
                        .fill(style.fill)
                        .frame(width: g.size.width * ratio)
                }
            }
            .frame(height: 10)
            .animation(reduceMotion ? nil : .easeOut(duration: 0.3), value: ratio)
        }
        .renderCard()
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(c.label ?? L("進捗"))
        .accessibilityValue(percent)
    }

    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    /// 色の段階(#140)。`raw` は丸める前の比率(1.2 = 120%)。
    /// - tone 省略: 従来どおり 100% 以上を negative
    /// - positive(目標の達成): 100% 以上を成功色
    /// - negative(予算の消化): 100% を超えたら警告色(ちょうど 100% は通常色)
    enum Level: Equatable { case normal, success, over }

    static func level(raw: Double, tone: String?) -> Level {
        switch tone {
        case "positive": raw >= 1 ? .success : .normal
        case "negative": raw > 1 ? .over : .normal
        default: raw >= 1 ? .over : .normal
        }
    }

    static func style(raw: Double, tone: String?) -> (text: Color, fill: AnyShapeStyle) {
        switch level(raw: raw, tone: tone) {
        case .normal: (RenderTheme.Colors.brandText, AnyShapeStyle(RenderTheme.brandGradient))
        case .success: (RenderTheme.Colors.positive, AnyShapeStyle(RenderTheme.Colors.positive))
        case .over: (RenderTheme.Colors.negative, AnyShapeStyle(RenderTheme.Colors.negative))
        }
    }

    private var resolvedRatio: Double {
        if let v = c.value { return v }
        guard let entityId = c.entity, let target = c.target, target > 0 else { return 0 }
        let rows = rt.filteredRecords(c)
        return Aggregator.aggregate(c.aggregate ?? "sum", field: c.field, over: rows) / target
    }
}
