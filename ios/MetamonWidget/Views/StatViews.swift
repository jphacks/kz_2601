import SwiftUI
import WidgetKit

// ホーム画面ウィジェットの見た目(small / medium / large)。WidgetKit の設定・タイムラインは MetamonWidget.swift。
// このフォルダ(Views/)は描画テスト(MetamonWidgetTests)にもコンパイルするので、Widget 型や #Preview を置かない。

/// 審査用の版(APP_STORE_REVIEW・#468)だけ「アプリを作る」を避けた文言にする(docs/store/wording.md)。通常の版は今のまま。
enum WidgetCopy {
    static var noApp: String {
        #if APP_STORE_REVIEW
        String(localized: "Tailor It で記録帳を用意するとここに数字が出ます")
        #else
        String(localized: "Tailor It でアプリを作るとここに数字が出ます")
        #endif
    }

    static var noAppShort: LocalizedStringKey {
        #if APP_STORE_REVIEW
        "記録帳を用意すると数字が出ます"
        #else
        "アプリを作ると数字が出ます"
        #endif
    }
}

struct StatEntry: TimelineEntry {
    let date: Date
    let snapshot: WidgetSnapshot?
}

// MARK: - Theme(本体 Theme.swift のブランド色を複製。Renderer を import できないため)

enum WidgetTheme {
    /// 藍(ライト #2E6A93 / ダーク #4E8DB8)・青緑(ライト #3E9A9C / ダーク #3B9E9F)。Renderer の SemanticPalette.brandA/brandB と同値(#102)
    static let brandA = dynamic(light: (46, 106, 147), dark: (78, 141, 184))
    static let brandB = dynamic(light: (62, 154, 156), dark: (59, 158, 159))

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
            case .accessoryRectangular: AccessoryRectangularView(snapshot: snapshot)
            case .accessoryCircular: AccessoryCircularView(stat: snapshot.stats[0])
            case .accessoryInline: AccessoryInlineView(stat: snapshot.stats[0])
            default: SmallStatView(snapshot: snapshot)
            }
        } else if family == .accessoryRectangular || family == .accessoryCircular || family == .accessoryInline {
            AccessoryEmptyView(family: family)
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
            // ウィジェットは端末の言語に従う(アプリ内の言語設定は共有しない・#44)
            Text(appName == nil
                 ? WidgetCopy.noApp
                 : String(localized: "「\(appName ?? "")」に集計(stat)の部品がありません"))
                .font(.caption)
                .foregroundStyle(.secondary)
                .lineLimit(3)
                .minimumScaleFactor(0.8)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
    }
}
