import WidgetKit
import SwiftUI

// Mac のウィジェット(#984)。デスクトップと通知センターに小・中・大。iOS(MetamonWidget)と同じく、本体が App Group に置いた
// `WidgetSnapshot`(集計済みの文字)を描くだけ。見た目は iOS の MetamonWidget/Views をそのまま使う(accessory は Mac に無い)。
// 更新: 本体の `WidgetCenter.reloadAllTimelines()`(`WidgetSnapshotWriter`)+ 30 分ごと。
// 押すと `tailorit://widget?app=<appId>` で本体のそのアプリを開く(`MacWidgetLink`)。

@main
struct MetamonMacWidgetBundle: WidgetBundle {
    var body: some Widget {
        MetamonMacStatWidget()
    }
}

struct MacStatProvider: TimelineProvider {
    func placeholder(in context: Context) -> StatEntry {
        StatEntry(date: .now, snapshot: .sample)
    }

    func getSnapshot(in context: Context, completion: @escaping (StatEntry) -> Void) {
        // ギャラリーではサンプル、置いたら実データ
        completion(StatEntry(date: .now, snapshot: context.isPreview ? .sample : WidgetSnapshot.load()))
    }

    func getTimeline(in context: Context, completion: @escaping (Timeline<StatEntry>) -> Void) {
        let entry = StatEntry(date: .now, snapshot: WidgetSnapshot.load())
        let next = Calendar.current.date(byAdding: .minute, value: 30, to: .now) ?? .now.addingTimeInterval(1800)
        completion(Timeline(entries: [entry], policy: .after(next)))
    }
}

struct MetamonMacStatWidget: Widget {
    let kind = "MetamonMacStatWidget"

    var body: some WidgetConfiguration {
        StaticConfiguration(kind: kind, provider: MacStatProvider()) { entry in
            MetamonWidgetView(entry: entry)
                .containerBackground(for: .widget) { WidgetTheme.background }
                .widgetURL(MacWidgetLink.url(appId: entry.snapshot?.appId))
        }
        .configurationDisplayName("Tailor It の数字")
        #if APP_STORE_REVIEW // 審査用の版は「アプリを作る」を避ける(#468・docs/store/wording.md)
        .description("用意した記録帳の集計(今月の支出など)をデスクトップと通知センターに表示します。")
        #else
        .description("作ったアプリの集計(今月の支出など)をデスクトップと通知センターに表示します。")
        #endif
        .supportedFamilies([.systemSmall, .systemMedium, .systemLarge])
    }
}

#Preview("Small", as: .systemSmall) {
    MetamonMacStatWidget()
} timeline: {
    StatEntry(date: .now, snapshot: .sample)
    StatEntry(date: .now, snapshot: nil)
}

#Preview("Medium", as: .systemMedium) {
    MetamonMacStatWidget()
} timeline: {
    StatEntry(date: .now, snapshot: .sample)
}

#Preview("Large", as: .systemLarge) {
    MetamonMacStatWidget()
} timeline: {
    StatEntry(date: .now, snapshot: .sample)
}
