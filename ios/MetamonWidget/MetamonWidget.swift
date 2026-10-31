import WidgetKit
import SwiftUI

// ホーム画面・ロック画面のウィジェット(ロック画面の accessory は #423)。本体が App Group に置いた `WidgetSnapshot`(集計済み文字列)を描画するだけ。
// Renderer/DSL には依存しない(WidgetKit は静的タイムライン + メモリ上限 30MB のため軽量に保つ)。
// 更新タイミング: 本体の `WidgetCenter.reloadAllTimelines()`(DSL 適用・レコード更新時)+ 30 分毎の自動リロード。

@main
struct MetamonWidgetBundle: WidgetBundle {
    var body: some Widget {
        MetamonStatWidget()
        TailorLiveActivity() // #423
    }
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
        #if APP_STORE_REVIEW // 審査用の版は「アプリを作る」を避ける(#468・docs/store/wording.md)
        .description("用意した記録帳の集計(今月の支出など)をホーム画面とロック画面に表示します。")
        #else
        .description("作ったアプリの集計(今月の支出など)をホーム画面とロック画面に表示します。")
        #endif
        .supportedFamilies([.systemSmall, .systemMedium, .accessoryRectangular, .accessoryCircular, .accessoryInline])
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
