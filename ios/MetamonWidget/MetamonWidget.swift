import AppIntents
import WidgetKit
import SwiftUI

// ホーム画面・ロック画面のウィジェット(ロック画面の accessory は #423)。本体が App Group に置いた `WidgetSnapshot`(集計済み文字列)を描画するだけ。
// Renderer/DSL には依存しない(WidgetKit は静的タイムライン + メモリ上限 30MB のため軽量に保つ)。
// 更新タイミング: 本体の `WidgetCenter.reloadAllTimelines()`(DSL 適用・レコード更新時・集中モードのフィルタの切り替え)+ 30 分毎の自動リロード。

@main
struct MetamonWidgetBundle: WidgetBundle {
    var body: some Widget {
        MetamonStatWidget()
        MetamonActionWidget() // #2238
        TailorLiveActivity() // #423
        if #available(iOS 18, *) { // コントロール(#1993)。iOS 17 では出さない
            OpenAppControl()
            RecordControl()
        }
    }
}

struct StatProvider: TimelineProvider {
    /// 置いてある数字。集中モードのフィルタで選んでいないアプリなら「集中モード中」(#1992・切り替えで本体が reload する)。
    private static func current() -> StatEntry {
        let snapshot = WidgetSnapshot.load()
        let focus = FocusFilterState.load(from: WidgetSnapshot.defaults)
        return StatEntry(date: .now, snapshot: snapshot, pausedByFocus: FocusFilterState.pausesWidget(snapshot, focus: focus))
    }

    func placeholder(in context: Context) -> StatEntry {
        StatEntry(date: .now, snapshot: .sample)
    }

    func getSnapshot(in context: Context, completion: @escaping (StatEntry) -> Void) {
        // ギャラリー(追加画面)ではサンプルを見せる。実配置時は実データ。
        completion(context.isPreview ? StatEntry(date: .now, snapshot: .sample) : Self.current())
    }

    func getTimeline(in context: Context, completion: @escaping (Timeline<StatEntry>) -> Void) {
        let entry = Self.current()
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
        .supportedFamilies([.systemSmall, .systemMedium, .systemLarge, .accessoryRectangular, .accessoryCircular, .accessoryInline])
    }
}

// MARK: - ボタンのウィジェット(#2238)

/// 設定: どの生成アプリのボタンを並べるか。一覧はコントロールと同じ `ControlCatalog`(本体が App Group に書く)
struct SelectWidgetAppIntent: WidgetConfigurationIntent {
    #if APP_STORE_REVIEW // 審査用の版は記録帳と言う(#247)
    static var title: LocalizedStringResource = "記録帳を選ぶ"
    @Parameter(title: "記録帳")
    var app: ControlAppEntity?
    #else
    static var title: LocalizedStringResource = "アプリを選ぶ"
    @Parameter(title: "アプリ")
    var app: ControlAppEntity?
    #endif
}

struct ActionProvider: AppIntentTimelineProvider {
    /// 選んだアプリのボタンと、押して記録した後に変わる数字。どちらも本体が App Group に書いたものを読むだけ
    private static func current(_ configuration: SelectWidgetAppIntent, family: WidgetFamily) -> ActionEntry {
        let appId = configuration.app?.id
        let catalog = ControlCatalog.load()
        let snapshot = WidgetSnapshot.load()
        let stat = appId != nil && snapshot?.appId == appId ? snapshot?.stats.first : nil
        return ActionEntry(date: .now,
                           appName: catalog.controlTitle(appId: appId),
                           buttons: catalog.widgetButtons(appId: appId, limit: WidgetActionLayout.limit(for: family)),
                           stat: stat)
    }

    func placeholder(in context: Context) -> ActionEntry {
        ActionEntry.sample
    }

    func snapshot(for configuration: SelectWidgetAppIntent, in context: Context) async -> ActionEntry {
        // ギャラリー(追加画面)ではサンプルを見せる。実配置時は実データ
        context.isPreview ? .sample : Self.current(configuration, family: context.family)
    }

    func timeline(for configuration: SelectWidgetAppIntent, in context: Context) async -> Timeline<ActionEntry> {
        let entry = Self.current(configuration, family: context.family)
        let next = Calendar.current.date(byAdding: .minute, value: 30, to: .now) ?? .now.addingTimeInterval(1800)
        return Timeline(entries: [entry], policy: .after(next))
    }
}

/// 押すと本体が開いて記録の入力画面が出る(コントロール #1993 と同じ扱い。ロック中は OS が解除を求める)。
/// 記録すると本体の `WidgetSnapshotWriter` が数字を書き直し、タイムラインが読み直される
struct MetamonActionWidget: Widget {
    let kind = "MetamonActionWidget"

    var body: some WidgetConfiguration {
        AppIntentConfiguration(kind: kind, intent: SelectWidgetAppIntent.self, provider: ActionProvider()) { entry in
            ActionButtonsView(entry: entry)
                .containerBackground(for: .widget) { WidgetTheme.background }
        }
        .configurationDisplayName("Tailor It のボタン")
        #if APP_STORE_REVIEW // 審査用の版は「アプリを作る」を避ける(#468・docs/store/wording.md)
        .description("用意した記録帳の記録の入力画面を、ホーム画面とロック画面のボタンから開きます。")
        #else
        .description("作ったアプリの記録の入力画面を、ホーム画面とロック画面のボタンから開きます。")
        #endif
        .supportedFamilies([.systemSmall, .systemMedium, .accessoryRectangular])
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

#Preview("Large", as: .systemLarge) {
    MetamonStatWidget()
} timeline: {
    StatEntry(date: .now, snapshot: .sample)
    StatEntry(date: .now, snapshot: WidgetSnapshot(appName: "家計簿", stats: WidgetSnapshot.sample.stats, updatedAt: .now,
                                                   recentTitle: "支出", recent: [], trend: nil))
    StatEntry(date: .now, snapshot: WidgetSnapshot(appName: "BMI", stats: [
        .init(key: "a", label: "最新の BMI", value: "21.4"),
        .init(key: "b", label: "目標まで", value: "-1.2 kg"),
    ], updatedAt: .now))
    StatEntry(date: .now, snapshot: nil)
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

#Preview("Action Small", as: .systemSmall) {
    MetamonActionWidget()
} timeline: {
    ActionEntry.sample
    ActionEntry(date: .now, appName: nil, buttons: [])
}

#Preview("Action Rectangular", as: .accessoryRectangular) {
    MetamonActionWidget()
} timeline: {
    ActionEntry.sample
}
