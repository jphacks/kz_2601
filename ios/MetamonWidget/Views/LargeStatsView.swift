import SwiftUI

/// systemLarge(#486): 上から stat 3 個(medium と同じ横並び)・直近 7 日の棒グラフ・最近の記録・フッター。
/// グラフ / 記録が無いスナップショット(古い版・式だけの stat)は stat を縦に大きく並べる。
struct LargeStatsView: View {
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    let snapshot: WidgetSnapshot

    /// 文字が大きい時はグラフを外し、記録の行を減らして収める。
    private var showsTrend: Bool { snapshot.trend != nil && !dynamicTypeSize.isAccessibilitySize }
    private var recentLimit: Int {
        if dynamicTypeSize.isAccessibilitySize { return dynamicTypeSize >= .accessibility3 ? 1 : 2 }
        if dynamicTypeSize >= .xxLarge { return showsTrend ? 2 : 3 }
        return showsTrend ? 4 : 5
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            if snapshot.trend == nil && snapshot.recent == nil {
                LargeStatList(stats: Array(snapshot.stats.prefix(3)), compact: dynamicTypeSize.isAccessibilitySize)
            } else {
                if dynamicTypeSize.isAccessibilitySize {
                    // 横 3 列だとラベルが切れるので、先頭 1 個だけ「ラベル 数字」の 1 行に
                    LargeStatList(stats: Array(snapshot.stats.prefix(1)), compact: true)
                } else {
                    StatsRow(stats: Array(snapshot.stats.prefix(3)), valueSize: 22)
                }
                if showsTrend, let trend = snapshot.trend {
                    TrendChart(trend: trend)
                }
                if let recent = snapshot.recent {
                    RecentList(title: snapshot.recentTitle, rows: Array(recent.prefix(recentLimit)), stacked: dynamicTypeSize.isAccessibilitySize)
                }
            }
            Spacer(minLength: 0)
            AppFooter(appName: snapshot.appName, updatedAt: snapshot.updatedAt)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
    }
}

/// stat を区切り線つきで横に並べる(medium / large 共通)。
struct StatsRow: View {
    let stats: [WidgetSnapshot.Stat]
    let valueSize: CGFloat

    var body: some View {
        HStack(alignment: .top, spacing: 0) {
            ForEach(Array(stats.enumerated()), id: \.element.id) { index, stat in
                if index > 0 {
                    Rectangle()
                        .fill(Color.primary.opacity(0.08))
                        .frame(width: 1)
                        .padding(.horizontal, 10)
                }
                VStack(alignment: .leading, spacing: 4) {
                    Text(stat.label)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                    StatValueText(value: stat.value, size: valueSize)
                }
                .frame(maxWidth: .infinity, alignment: .leading)
            }
        }
        .fixedSize(horizontal: false, vertical: true)
    }
}

/// グラフ・記録が無い時: stat を 1 行ずつ大きく。
private struct LargeStatList: View {
    let stats: [WidgetSnapshot.Stat]
    /// 文字が大きい時は数字と間隔を詰める。
    let compact: Bool

    var body: some View {
        VStack(alignment: .leading, spacing: compact ? 6 : 14) {
            ForEach(Array(stats.enumerated()), id: \.element.id) { index, stat in
                if index > 0 { Divider() }
                VStack(alignment: .leading, spacing: 4) {
                    Text(stat.label)
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                    StatValueText(value: stat.value, size: compact ? 26 : 34)
                }
            }
        }
    }
}

/// 直近 7 日の棒グラフ。今日の棒だけブランドのグラデーション。
struct TrendChart: View {
    let trend: WidgetSnapshot.Trend

    private var maxValue: Double { max(trend.points.map { abs($0.value) }.max() ?? 0, 0) }

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack(alignment: .firstTextBaseline, spacing: 4) {
                Text("直近 7 日")
                    .font(.caption.weight(.semibold))
                Text(verbatim: "· " + (trend.metric ?? String(localized: "件数")))
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
                Spacer(minLength: 4)
                Text(trend.total)
                    .font(.caption.weight(.semibold).monospacedDigit())
                    .lineLimit(1)
                    .minimumScaleFactor(0.7)
            }
            HStack(alignment: .bottom, spacing: 6) {
                ForEach(Array(trend.points.enumerated()), id: \.offset) { index, point in
                    let isToday = index == trend.points.count - 1
                    VStack(spacing: 3) {
                        GeometryReader { geo in
                            let ratio = maxValue > 0 ? abs(point.value) / maxValue : 0
                            VStack(spacing: 0) {
                                Spacer(minLength: 0)
                                RoundedRectangle(cornerRadius: 3, style: .continuous)
                                    .fill(isToday ? AnyShapeStyle(WidgetTheme.brandGradient) : AnyShapeStyle(WidgetTheme.brandA.opacity(0.35)))
                                    .frame(height: max(3, geo.size.height * ratio))
                            }
                        }
                        Text(point.day, format: .dateTime.weekday(.narrow))
                            .font(.caption2)
                            .foregroundStyle(isToday ? AnyShapeStyle(.primary) : AnyShapeStyle(.secondary))
                            .lineLimit(1)
                    }
                    .frame(maxWidth: .infinity)
                }
            }
            .frame(height: 56)
            .accessibilityElement(children: .ignore)
            .accessibilityLabel(Text("直近 7 日"))
            .accessibilityValue(Text(trend.total))
        }
    }
}

/// 最近の記録(新しい順)。0 件なら案内を出す。
struct RecentList: View {
    let title: String?
    let rows: [WidgetSnapshot.Row]
    /// 文字が大きい時は見出しを 1 行目、日付と値を 2 行目に分ける。
    var stacked = false

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack(alignment: .firstTextBaseline, spacing: 4) {
                Text("最近の記録")
                    .font(.caption.weight(.semibold))
                if let title {
                    Text(verbatim: "· \(title)")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                }
            }
            if rows.isEmpty {
                Text("まだ記録がありません")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            } else {
                VStack(spacing: 0) {
                    ForEach(Array(rows.enumerated()), id: \.element.id) { index, row in
                        if index > 0 {
                            Rectangle().fill(Color.primary.opacity(0.08)).frame(height: 1)
                        }
                        RecentRow(row: row, stacked: stacked)
                            .padding(.vertical, 4)
                    }
                }
            }
        }
    }
}

private struct RecentRow: View {
    let row: WidgetSnapshot.Row
    let stacked: Bool

    var body: some View {
        if stacked {
            VStack(alignment: .leading, spacing: 2) {
                Text(row.title)
                    .font(.subheadline)
                    .lineLimit(1)
                HStack(alignment: .firstTextBaseline, spacing: 8) {
                    Spacer(minLength: 0)
                    trailing
                }
            }
        } else {
            HStack(alignment: .firstTextBaseline, spacing: 8) {
                Text(row.title)
                    .font(.subheadline)
                    .lineLimit(1)
                    .frame(maxWidth: .infinity, alignment: .leading)
                trailing
            }
        }
    }

    @ViewBuilder private var trailing: some View {
        if let detail = row.detail {
            Text(detail)
                .font(.caption)
                .foregroundStyle(.secondary)
                .lineLimit(1)
                .layoutPriority(1)
        }
        if let value = row.value {
            Text(value)
                .font(.subheadline.weight(.semibold).monospacedDigit())
                .lineLimit(1)
                .layoutPriority(2)
        }
    }
}
