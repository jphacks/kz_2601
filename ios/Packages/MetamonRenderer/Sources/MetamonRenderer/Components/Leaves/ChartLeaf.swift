import SwiftUI
import Charts
import Accessibility
import MetamonDSL

/// `chart`: groupBy で束ねた集計を bar|line|pie で描く(Swift Charts)。
/// 押した棒・点・扇の値を上に出し、VoiceOver には要約とグラフの詳細を渡す(UX 点検 L4)。
/// Mac はポインタを載せた所に一番近い棒・点・扇の値を吹き出しで出す(#945・押す代わり)。
struct ChartLeaf: View {
    @Environment(\.dslRuntime) private var rt
    let c: Component
    /// 押している棒・点の見出し(bar / line)
    @State private var selectedLabel: String?
    /// 押している扇の累積値(pie)
    @State private var selectedAngle: Double?
    /// ポインタの位置(Mac のホバーの吹き出し)
    @State private var hoverPoint: CGPoint?

    var body: some View {
        let entityId = c.entity ?? ""
        // 日付の見出し(「3月」「3/4週」)は UI の言語で
        let buckets = Self.buckets(c, rt: rt, locale: LanguagePreference.shared.locale)
        VStack(alignment: .leading, spacing: 8) {
            if let title = c.title { Text(title).font(RenderTheme.Fonts.cardTitle) }
            if buckets.isEmpty {
                // 記録すると出ることを伝える(L4)
                EmptyPlaceholder(icon: "chart.bar", emptyText: L("まだ記録がありません。記録するとグラフになります"), state: rt.emptyState(for: entityId)) {
                    Task { await rt.load(entities: [entityId], force: true) }
                }
            } else {
                let selected = selection(in: buckets)
                #if !os(macOS)
                readout(selected) // Mac はポインタの横の吹き出し
                #endif
                Chart(buckets) { b in
                    switch c.kind {
                    case "line":
                        LineMark(x: .value("x", b.label), y: .value("y", b.value))
                            .foregroundStyle(RenderTheme.Colors.brandA)
                            .lineStyle(StrokeStyle(lineWidth: 2.5, lineCap: .round))
                            .interpolationMethod(.catmullRom)
                            .accessibilityHidden(true) // 読み上げは点ごと(PointMark)
                        AreaMark(x: .value("x", b.label), y: .value("y", b.value))
                            .foregroundStyle(RenderTheme.brandWash)
                            .interpolationMethod(.catmullRom)
                            .accessibilityHidden(true)
                        PointMark(x: .value("x", b.label), y: .value("y", b.value))
                            .foregroundStyle(RenderTheme.Colors.brandA)
                            .symbolSize(selected?.id == b.id ? 90 : 40)
                            .accessibilityLabel(b.label)
                            .accessibilityValue(ChartReadout.format(b.value))
                    case "pie":
                        SectorMark(angle: .value("y", b.value), innerRadius: .ratio(0.55), angularInset: 1.5)
                            .cornerRadius(4)
                            .foregroundStyle(by: .value("x", b.label))
                            .opacity(selected == nil || selected?.id == b.id ? 1 : 0.4)
                            .accessibilityLabel(b.label)
                            .accessibilityValue(ChartReadout.format(b.value))
                    default:
                        BarMark(x: .value("x", b.label), y: .value("y", b.value))
                            .foregroundStyle(RenderTheme.brandGradient)
                            .cornerRadius(6)
                            .opacity(selected == nil || selected?.id == b.id ? 1 : 0.4)
                            .accessibilityLabel(b.label)
                            .accessibilityValue(ChartReadout.format(b.value))
                    }
                }
                .chartForegroundStyleScale(range: RenderTheme.Colors.chartPalette)
                // #131: 長いカテゴリ名で x 軸ラベルが重なっていたので、短く切って衝突時は間引く
                .chartXAxis {
                    AxisMarks { v in
                        AxisValueLabel(collisionResolution: .greedy) {
                            Text(Self.axisLabel(v.as(String.self) ?? "")).font(RenderTheme.Fonts.legend).lineLimit(1)
                        }
                    }
                }
                .chartYAxis { AxisMarks { AxisGridLine().foregroundStyle(RenderTheme.Colors.hairline); AxisValueLabel().font(RenderTheme.Fonts.legend) } }
                // 凡例は上(左下に置くと左下のマイク FAB に隠れる)
                .chartLegend(position: .top, alignment: .leading, spacing: RenderTheme.Spacing.sm)
                .font(RenderTheme.Fonts.legend)
                // グリッドのセル内ではセルの高さに合わせて伸縮、縦積みでは ideal(200)で描く
                .frame(minHeight: 120, idealHeight: 200, maxHeight: .infinity)
                #if os(macOS)
                // ポインタを載せている間だけ値を出す(#945)
                .chartOverlay { proxy in
                    GeometryReader { geo in
                        Rectangle().fill(.clear).contentShape(Rectangle())
                            .onContinuousHover { phase in hover(phase, proxy: proxy, plot: proxy.plotFrame.map { geo[$0] }, buckets: buckets) }
                            .overlay(alignment: .topLeading) { tooltip(selected, in: geo.size) }
                    }
                }
                #else
                // 押している間だけ値を出す(L4)
                .modifier(ChartSelection(pie: c.kind == "pie", label: $selectedLabel, angle: $selectedAngle))
                #endif
                .accessibilityChartDescriptor(ChartDescriptor(title: c.title, buckets: buckets))
            }
        }
        .frame(maxHeight: .infinity, alignment: .top)
        .renderCard()
    }

    /// 押している棒・点・扇
    private func selection(in buckets: [Aggregator.Bucket]) -> Aggregator.Bucket? {
        if c.kind == "pie" {
            return selectedAngle.flatMap { ChartReadout.index(atCumulative: $0, values: buckets.map(\.value)) }.map { buckets[$0] }
        }
        return selectedLabel.flatMap { label in buckets.first { $0.label == label } }
    }

    /// 押した所の値。押していない時も高さを取っておく(押すたびにグラフが上下しない)
    private func readout(_ bucket: Aggregator.Bucket?) -> some View {
        Text(bucket.map { ChartReadout.line(label: $0.label, value: $0.value) } ?? " ")
            .font(RenderTheme.Fonts.legend.weight(.semibold).monospacedDigit())
            .foregroundStyle(RenderTheme.Colors.brandA)
            .lineLimit(1)
            .accessibilityHidden(bucket == nil)
    }

    #if os(macOS)
    /// ポインタに一番近い棒・点(x の見出し)/ 扇(累積値)を選ぶ。外れたら消す
    private func hover(_ phase: HoverPhase, proxy: ChartProxy, plot: CGRect?, buckets: [Aggregator.Bucket]) {
        guard case .active(let point) = phase, let plot else {
            selectedLabel = nil
            selectedAngle = nil
            hoverPoint = nil
            return
        }
        hoverPoint = point
        if c.kind == "pie" {
            let center = CGPoint(x: plot.midX, y: plot.midY)
            selectedAngle = ChartReadout.cumulative(at: point, center: center, radius: min(plot.width, plot.height) / 2,
                                                    total: buckets.reduce(0) { $0 + max($1.value, 0) })
        } else {
            let xs = buckets.map { b in proxy.position(forX: b.label).map { $0 + plot.minX } }
            selectedLabel = ChartReadout.nearestIndex(toX: point.x, positions: xs).map { buckets[$0].label }
        }
    }

    /// ポインタの右上に出す値の吹き出し(はみ出す時は左へ)。読み上げは棒・点・扇の label / value で足りるので隠す
    @ViewBuilder
    private func tooltip(_ bucket: Aggregator.Bucket?, in size: CGSize) -> some View {
        if let bucket, let p = hoverPoint {
            let text = ChartReadout.line(label: bucket.label, value: bucket.value)
            Text(text)
                .font(RenderTheme.Fonts.legend.weight(.semibold).monospacedDigit())
                .foregroundStyle(RenderTheme.Colors.brandA)
                .lineLimit(1)
                .padding(.horizontal, 8)
                .padding(.vertical, 4)
                .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 6))
                .fixedSize()
                .alignmentGuide(.leading) { d in p.x + 12 + d.width > size.width ? -(p.x - 12 - d.width) : -(p.x + 12) }
                .alignmentGuide(.top) { d in -max(0, p.y - d.height - 8) }
                .allowsHitTesting(false)
                .accessibilityHidden(true)
        }
    }
    #endif

    /// x 軸ラベル: 6 文字を超えたら 5 文字 + …(凡例・VoiceOver は元の文字列のまま)
    /// 棒・点・扇の元(groupBy の集計)。適合テストの描画の木(`RenderTree`)も同じ値を使う。
    @MainActor
    static func buckets(_ c: Component, rt: any DSLRuntime, locale: Locale? = nil) -> [Aggregator.Bucket] {
        let entity = rt.dsl.entity(c.entity ?? "")
        let groupField = c.groupBy.flatMap { entity?.field($0) }
        // ref の groupBy は id で束ねるので、軸・凡例は参照先の見出しにする(#235)
        return Aggregator.buckets(c, entity: entity, records: rt.filteredRecords(c),
                                  label: groupField?.type == .ref ? { rt.display(.string($0), field: groupField) } : nil,
                                  locale: locale)
    }

    static func axisLabel(_ s: String) -> String {
        s.count > 6 ? String(s.prefix(5)) + "…" : s
    }
}

#if !os(macOS)
/// 押している棒・点(x の見出し)/ 扇(累積値)を受け取る(Mac はホバー・`ChartLeaf.hover`)。
private struct ChartSelection: ViewModifier {
    let pie: Bool
    @Binding var label: String?
    @Binding var angle: Double?

    @ViewBuilder
    func body(content: Content) -> some View {
        if pie {
            content.chartAngleSelection(value: $angle)
        } else {
            content.chartXSelection(value: $label)
        }
    }
}
#endif

/// VoiceOver のグラフの詳細(オーディオグラフ)と要約(L4)。
private struct ChartDescriptor: AXChartDescriptorRepresentable {
    let title: String?
    let buckets: [Aggregator.Bucket]

    func makeChartDescriptor() -> AXChartDescriptor {
        ChartReadout.descriptor(title: title, buckets: buckets)
    }
}
