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
        // tone 指定時は実比率(112%)。桁が暴れないよう 999% で頭打ち
        let shown = c.tone == nil ? ratio : min(max(0, raw), 9.99)
        let percent = String(format: "%.0f%%", shown * 100) + (c.tone != nil && raw > 9.99 ? "+" : "")
        // area が 1 行しか無いとラベル + バーの 2 段がセルに収まらず、下端が隣のセルに隠れていた(#157)。
        // 高さが足りない時はラベル・バー・% を 1 段に並べた低いカードに切り替える(セル内スクロールにはしない)
        ViewThatFits(in: .vertical) {
            regular(ratio: ratio, percent: percent, style: style)
            compact(ratio: ratio, percent: percent, style: style)
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(c.label ?? L("進捗"))
        .accessibilityValue(percent)
    }

    private typealias Style = (text: Color, fill: AnyShapeStyle)

    private func regular(ratio: Double, percent: String, style: Style) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            if let label = c.label {
                HStack {
                    Text(label).font(RenderTheme.Fonts.statLabel).foregroundStyle(.secondary)
                    Spacer()
                    percentText(percent, style: style)
                }
            }
            bar(ratio, style: style).frame(height: 10)
        }
        .renderCard()
    }

    /// 1 段版: [ラベル  ━━━━━  42%]。縦の余白も詰める。
    private func compact(ratio: Double, percent: String, style: Style) -> some View {
        HStack(spacing: RenderTheme.Spacing.sm) {
            if let label = c.label {
                Text(label).font(RenderTheme.Fonts.statLabel).foregroundStyle(.secondary)
                    .lineLimit(1)
                    .minimumScaleFactor(0.8)
                    .layoutPriority(1)
            }
            bar(ratio, style: style).frame(minWidth: 48).frame(height: 8)
            percentText(percent, style: style)
        }
        .padding(.horizontal, RenderTheme.Spacing.sm)
        .renderCard(padding: RenderTheme.Spacing.sm)
    }

    private func percentText(_ percent: String, style: Style) -> some View {
        Text(percent)
            .font(.system(.subheadline, design: .rounded).weight(.semibold).monospacedDigit())
            .foregroundStyle(style.text)
            .fixedSize()
    }

    private func bar(_ ratio: Double, style: Style) -> some View {
        GeometryReader { g in
            ZStack(alignment: .leading) {
                Capsule().fill(RenderTheme.Colors.surfaceElevated)
                Capsule()
                    .fill(style.fill)
                    .frame(width: g.size.width * ratio)
            }
        }
        .animation(reduceMotion ? nil : .easeOut(duration: 0.3), value: ratio)
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
