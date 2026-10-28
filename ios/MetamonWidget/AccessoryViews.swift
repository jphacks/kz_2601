import SwiftUI
import WidgetKit

// ロック画面のウィジェット(accessory・#423)。ホーム画面と同じ `WidgetSnapshot` の stat を出す。
// ロック画面は単色(vibrant)で描かれるので色に頼らず、文字の太さと大きさで区別する。

/// accessoryRectangular: アプリ名 + stat 最大 2 個。
struct AccessoryRectangularView: View {
    let snapshot: WidgetSnapshot

    var body: some View {
        VStack(alignment: .leading, spacing: 1) {
            Text(snapshot.appName)
                .font(.caption2.weight(.semibold))
                .widgetAccentable()
                .lineLimit(1)
            ForEach(snapshot.stats.prefix(2)) { stat in
                HStack(spacing: 4) {
                    Text(stat.label)
                        .font(.caption2)
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                    Spacer(minLength: 2)
                    Text(stat.value ?? "—")
                        .font(.headline.monospacedDigit())
                        .lineLimit(1)
                        .minimumScaleFactor(0.6)
                }
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}

/// accessoryCircular: 先頭の stat を丸の中に。
struct AccessoryCircularView: View {
    let stat: WidgetSnapshot.Stat

    var body: some View {
        ZStack {
            AccessoryWidgetBackground()
            VStack(spacing: 0) {
                Text(stat.value ?? "—")
                    .font(.system(size: 17, weight: .bold, design: .rounded).monospacedDigit())
                    .lineLimit(1)
                    .minimumScaleFactor(0.4)
                Text(stat.label)
                    .font(.system(size: 9))
                    .lineLimit(1)
                    .minimumScaleFactor(0.6)
            }
            .padding(4)
        }
    }
}

/// accessoryInline: 時計の上の 1 行。
struct AccessoryInlineView: View {
    let stat: WidgetSnapshot.Stat

    var body: some View {
        Text("\(stat.label) \(stat.value ?? "—")")
    }
}

/// データ無し(アプリ未生成 / stat が無い)。
struct AccessoryEmptyView: View {
    let family: WidgetFamily

    var body: some View {
        switch family {
        case .accessoryCircular:
            ZStack {
                AccessoryWidgetBackground()
                Image(systemName: "waveform")
                    .font(.system(size: 18, weight: .semibold))
            }
        case .accessoryInline:
            Text("Tailor It")
        default:
            VStack(alignment: .leading, spacing: 2) {
                Text("Tailor It").font(.caption2.weight(.semibold))
                Text("アプリを作ると数字が出ます")
                    .font(.caption2)
                    .foregroundStyle(.secondary)
                    .lineLimit(2)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
        }
    }
}

// MARK: - Previews

#Preview("Rectangular", as: .accessoryRectangular) {
    MetamonStatWidget()
} timeline: {
    StatEntry(date: .now, snapshot: .sample)
    StatEntry(date: .now, snapshot: nil)
}

#Preview("Circular", as: .accessoryCircular) {
    MetamonStatWidget()
} timeline: {
    StatEntry(date: .now, snapshot: .sample)
}

#Preview("Inline", as: .accessoryInline) {
    MetamonStatWidget()
} timeline: {
    StatEntry(date: .now, snapshot: .sample)
}
