import WidgetKit
import SwiftUI

// ホーム画面ウィジェット。本体が App Group に置いた `WidgetSnapshot`(集計済み文字列)を描画するだけ。
// Renderer/DSL には依存しない(WidgetKit は静的タイムライン + メモリ上限 30MB のため軽量に保つ)。
// 更新タイミング: 本体の `WidgetCenter.reloadAllTimelines()`(DSL 適用・レコード更新時)+ 30 分毎の自動リロード。

@main
struct MetamonWidgetBundle: WidgetBundle {
    var body: some Widget {
        MetamonStatWidget()
    }
}

struct StatEntry: TimelineEntry {
    let date: Date
    let snapshot: WidgetSnapshot?
}

struct StatProvider: TimelineProvider {
    func placeholder(in context: Context) -> StatEntry {
        StatEntry(date: .now, snapshot: .sample)
    }

    func getSnapshot(in context: Context, completion: @escaping (StatEntry) -> Void) {
        // ギャラリー(追加画面)ではサンプルを見せる。実配置時は実データ。
        completion(StatEntry(date: .now, snapshot: context.isPreview ? .sample : WidgetSnapshot.load()))
    }

    func getTimeline(in context: Context, completion: @escaping (Timeline<StatEntry>) -> Void) {
        let entry = StatEntry(date: .now, snapshot: WidgetSnapshot.load())
        let next = Calendar.current.date(byAdding: .minute, value: 30, to: .now) ?? .now.addingTimeInterval(1800)
        completion(Timeline(entries: [entry], policy: .after(next)))
    }
}

struct MetamonStatWidget: Widget {
    let kind = "MetamonStatWidget"

    var body: some WidgetConfiguration {
        StaticConfiguration(kind: kind, provider: StatProvider()) { entry in
            MetamonWidgetView(entry: entry)
                .containerBackground(for: .widget) { WidgetTheme.background }
                .widgetURL(URL(string: "metamon://widget"))
        }
        .configurationDisplayName("Tailor It の数字")
        .description("作ったアプリの集計(今月の支出など)をホーム画面に表示します。")
        .supportedFamilies([.systemSmall, .systemMedium])
    }
}

// MARK: - Theme(本体 Theme.swift のブランド色を複製。Renderer を import できないため)

enum WidgetTheme {
    /// 藍(ライト #2E6A93 / ダーク #6FA8CF)・青緑(ライト #3E9A9C / ダーク #5FC0BE)。Renderer の SemanticPalette.brandA/brandB と同値(#102)
    static let brandA = dynamic(light: (46, 106, 147), dark: (111, 168, 207))
    static let brandB = dynamic(light: (62, 154, 156), dark: (95, 192, 190))

    private static func dynamic(light: (Int, Int, Int), dark: (Int, Int, Int)) -> Color {
        Color(UIColor { trait in
            let c = trait.userInterfaceStyle == .dark ? dark : light
            return UIColor(red: CGFloat(c.0) / 255, green: CGFloat(c.1) / 255, blue: CGFloat(c.2) / 255, alpha: 1)
        })
    }

    static let brandGradient = LinearGradient(
        colors: [brandA, brandB], startPoint: .topLeading, endPoint: .bottomTrailing
    )

    /// カード背景: システム背景 + ブランドグラデーションの薄い滲み(ライト/ダーク両対応)。
    static var background: some View {
        ZStack {
            Rectangle().fill(.background)
            LinearGradient(
                colors: [brandA.opacity(0.22), brandB.opacity(0.16)],
                startPoint: .topLeading, endPoint: .bottomTrailing
            )
            RadialGradient(
                colors: [brandB.opacity(0.25), .clear],
                center: .init(x: 1.0, y: 1.0), startRadius: 0, endRadius: 180
            )
        }
    }
}

// MARK: - Views

struct MetamonWidgetView: View {
    @Environment(\.widgetFamily) private var family
    let entry: StatEntry

    var body: some View {
        if let snapshot = entry.snapshot, !snapshot.stats.isEmpty {
            switch family {
            case .systemMedium: MediumStatsView(snapshot: snapshot)
            default: SmallStatView(snapshot: snapshot)
            }
        } else {
            EmptyStateView(appName: entry.snapshot?.appName)
        }
    }
}

/// systemSmall: stat 1 個を大きく。
struct SmallStatView: View {
    let snapshot: WidgetSnapshot

    var body: some View {
        let stat = snapshot.stats[0]
        VStack(alignment: .leading, spacing: 4) {
            Text(stat.label)
                .font(.caption)
                .foregroundStyle(.secondary)
                .lineLimit(1)
            StatValueText(value: stat.value, size: 30)
            Spacer(minLength: 0)
            AppFooter(appName: snapshot.appName, updatedAt: nil)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
    }
}

/// systemMedium: stat 最大 3 個を横並び。
struct MediumStatsView: View {
    let snapshot: WidgetSnapshot

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack(alignment: .top, spacing: 0) {
                ForEach(Array(snapshot.stats.prefix(3).enumerated()), id: \.element.id) { index, stat in
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
                        StatValueText(value: stat.value, size: 24)
                    }
                    .frame(maxWidth: .infinity, alignment: .leading)
                }
            }
            Spacer(minLength: 0)
            AppFooter(appName: snapshot.appName, updatedAt: snapshot.updatedAt)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
    }
}

/// 数値。`nil`(レコード未取得)は「—」。
struct StatValueText: View {
    let value: String?
    let size: CGFloat

    var body: some View {
        Text(value ?? "—")
            .font(.system(size: size, weight: .bold, design: .rounded).monospacedDigit())
            .lineLimit(1)
            .minimumScaleFactor(0.5)
            .foregroundStyle(value == nil ? AnyShapeStyle(.secondary) : AnyShapeStyle(.primary))
            .contentTransition(.numericText())
    }
}

/// 下部: ブランドマーク + アプリ名(+ 更新時刻)。
struct AppFooter: View {
    let appName: String
    let updatedAt: Date?

    var body: some View {
        HStack(spacing: 6) {
            ZStack {
                Circle().fill(WidgetTheme.brandGradient)
                Image(systemName: "waveform")
                    .font(.system(size: 8, weight: .semibold))
                    .foregroundStyle(.white)
            }
            .frame(width: 16, height: 16)
            Text(appName)
                .font(.caption2.weight(.semibold))
                .lineLimit(1)
            Spacer(minLength: 0)
            if let updatedAt {
                Text(updatedAt, style: .time)
                    .font(.caption2)
                    .foregroundStyle(.tertiary)
            }
        }
    }
}

/// データ無し(アプリ未生成 / stat 部品が無い)。
struct EmptyStateView: View {
    let appName: String?

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            ZStack {
                Circle().fill(WidgetTheme.brandGradient)
                Image(systemName: "waveform")
                    .font(.system(size: 14, weight: .semibold))
                    .foregroundStyle(.white)
            }
            .frame(width: 30, height: 30)
            Spacer(minLength: 0)
            Text(appName == nil
                 ? "Tailor It でアプリを作るとここに数字が出ます"
                 : "「\(appName ?? "")」に集計(stat)の部品がありません")
                .font(.caption)
                .foregroundStyle(.secondary)
                .lineLimit(3)
                .minimumScaleFactor(0.8)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
    }
}

// MARK: - Previews

#Preview("Small", as: .systemSmall) {
    MetamonStatWidget()
} timeline: {
    StatEntry(date: .now, snapshot: .sample)
    StatEntry(date: .now, snapshot: nil)
}

#Preview("Medium", as: .systemMedium) {
    MetamonStatWidget()
} timeline: {
    StatEntry(date: .now, snapshot: .sample)
    StatEntry(date: .now, snapshot: WidgetSnapshot(appName: "筋トレ記録", stats: [
        .init(key: "a", label: "今週の回数", value: "4 件"),
    ], updatedAt: .now))
}
