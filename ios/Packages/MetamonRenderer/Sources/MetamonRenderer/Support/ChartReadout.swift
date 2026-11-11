import Foundation
import CoreGraphics
import Accessibility
import MetamonDSL

/// グラフの値の読み取り(UX 点検 L4): 押した棒・点・扇の値と、VoiceOver の要約。View から切り離してテストする。
enum ChartReadout {
    /// 値の表示。整数はそのまま(3 桁区切り)、小数は 1 桁まで
    static func format(_ value: Double, locale: Locale = .current) -> String {
        value.formatted(.number.precision(.fractionLength(0...1)).locale(locale))
    }

    /// 押した所の読み上げ・表示「3月: 12」
    static func line(label: String, value: Double, locale: Locale = .current) -> String {
        "\(label): \(format(value, locale: locale))"
    }

    /// 扇グラフの選択(`chartAngleSelection` は先頭からの累積値を返す)→ その扇の位置
    static func index(atCumulative value: Double, values: [Double]) -> Int? {
        var sum = 0.0
        for (i, v) in values.enumerated() {
            sum += v
            if value <= sum { return i }
        }
        return values.isEmpty ? nil : values.count - 1
    }

    /// Mac のホバー(#945): ポインタの x に一番近い棒・点の位置。`positions` は各見出しの x(描けない見出しは nil)
    static func nearestIndex(toX x: CGFloat, positions: [CGFloat?]) -> Int? {
        var best: (index: Int, distance: CGFloat)?
        for (i, p) in positions.enumerated() {
            guard let p else { continue }
            let d = abs(p - x)
            if best == nil || d < best!.distance { best = (i, d) }
        }
        return best?.index
    }

    /// Mac のホバー(#945): 扇グラフの中の点 → 12 時から時計回りの累積値(`index(atCumulative:)` に渡す)。
    /// 円の外・中心(角度が決まらない)・合計 0 は nil
    static func cumulative(at point: CGPoint, center: CGPoint, radius: CGFloat, total: Double) -> Double? {
        let dx = point.x - center.x, dy = point.y - center.y
        let distance = (dx * dx + dy * dy).squareRoot()
        guard total > 0, distance > 0, distance <= radius else { return nil }
        var angle = atan2(Double(dx), Double(-dy)) // 上が 0・時計回り(y は下向き)
        if angle < 0 { angle += 2 * .pi }
        return angle / (2 * .pi) * total
    }

    static func summary(_ buckets: [Aggregator.Bucket], locale: Locale = .current) -> String {
        summary(labels: buckets.map(\.label), values: buckets.map(\.value), locale: locale)
    }

    /// VoiceOver の要約「4 項目。最大は 3月 の 12、最小は 1月 の 2」。1 項目なら「1 項目。3月 の 12」
    static func summary(labels: [String], values: [Double], locale: Locale = .current) -> String {
        let points = zip(labels, values).map { (label: $0, value: $1) }
        guard let maxB = points.max(by: { $0.value < $1.value }),
              let minB = points.min(by: { $0.value < $1.value }) else { return L("データがありません") }
        let count = points.count
        if count == 1 {
            return L("\(count) 項目。\(maxB.label) の \(format(maxB.value, locale: locale))")
        }
        return L("\(count) 項目。最大は \(maxB.label) の \(format(maxB.value, locale: locale))、最小は \(minB.label) の \(format(minB.value, locale: locale))")
    }

    /// VoiceOver のオーディオグラフ・グラフの詳細(`accessibilityChartDescriptor`)
    static func descriptor(title: String?, buckets: [Aggregator.Bucket]) -> AXChartDescriptor {
        let labels = buckets.map(\.label)
        let values = buckets.map(\.value)
        let x = AXCategoricalDataAxisDescriptor(title: L("項目"), categoryOrder: labels)
        let lower = min(0, values.min() ?? 0)
        let upper = max(values.max() ?? 0, lower + 1)
        let y = AXNumericDataAxisDescriptor(title: L("値"), range: lower...upper, gridlinePositions: []) { format($0) }
        let series = AXDataSeriesDescriptor(name: title ?? L("グラフ"), isContinuous: false,
                                            dataPoints: buckets.map { AXDataPoint(x: $0.label, y: $0.value) })
        return AXChartDescriptor(title: title, summary: summary(buckets), xAxis: x, yAxis: y, additionalAxes: [], series: [series])
    }
}
