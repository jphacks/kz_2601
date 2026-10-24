import SwiftUI
import Charts
import MetamonDSL

/// `chart`: groupBy で束ねた集計を bar|line|pie で描く(Swift Charts)。
struct ChartLeaf: View {
    @Environment(\.dslRuntime) private var rt
    let c: Component

    var body: some View {
        let entityId = c.entity ?? ""
        let entity = rt.dsl.entity(entityId)
        let rows = rt.filteredRecords(c)
        let buckets = Aggregator.buckets(c, entity: entity, records: rows)
        VStack(alignment: .leading, spacing: 8) {
            if let title = c.title { Text(title).font(RenderTheme.Fonts.cardTitle) }
            if buckets.isEmpty {
                EmptyPlaceholder(icon: "chart.bar", emptyText: "データがありません", state: rt.emptyState(for: entityId)) {
                    Task { await rt.load(entities: [entityId], force: true) }
                }
            } else {
                Chart(buckets) { b in
                    switch c.kind {
                    case "line":
                        LineMark(x: .value("x", b.label), y: .value("y", b.value))
                            .foregroundStyle(RenderTheme.Colors.brandA)
                            .lineStyle(StrokeStyle(lineWidth: 2.5, lineCap: .round))
                            .interpolationMethod(.catmullRom)
                        AreaMark(x: .value("x", b.label), y: .value("y", b.value))
                            .foregroundStyle(RenderTheme.brandWash)
                            .interpolationMethod(.catmullRom)
                        PointMark(x: .value("x", b.label), y: .value("y", b.value))
                            .foregroundStyle(RenderTheme.Colors.brandA)
                            .symbolSize(40)
                    case "pie":
                        SectorMark(angle: .value("y", b.value), innerRadius: .ratio(0.55), angularInset: 1.5)
                            .cornerRadius(4)
                            .foregroundStyle(by: .value("x", b.label))
                    default:
                        BarMark(x: .value("x", b.label), y: .value("y", b.value))
                            .foregroundStyle(RenderTheme.brandGradient)
                            .cornerRadius(6)
                    }
                }
                .chartForegroundStyleScale(range: RenderTheme.Colors.chartPalette)
                .chartXAxis { AxisMarks { AxisValueLabel().font(RenderTheme.Fonts.legend) } }
                .chartYAxis { AxisMarks { AxisGridLine().foregroundStyle(RenderTheme.Colors.hairline); AxisValueLabel().font(RenderTheme.Fonts.legend) } }
                // 凡例は上(左下に置くと左下のマイク FAB に隠れる)
                .chartLegend(position: .top, alignment: .leading, spacing: RenderTheme.Spacing.sm)
                .font(RenderTheme.Fonts.legend)
                // グリッドのセル内ではセルの高さに合わせて伸縮、縦積みでは ideal(200)で描く
                .frame(minHeight: 120, idealHeight: 200, maxHeight: .infinity)
            }
        }
        .frame(maxHeight: .infinity, alignment: .top)
        .renderCard()
    }
}
