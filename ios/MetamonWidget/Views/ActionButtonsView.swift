import AppIntents
import SwiftUI
import WidgetKit

// ボタンを置くウィジェット(#2238)の見た目。WidgetKit の設定・タイムラインは MetamonWidget.swift。
// このフォルダ(Views/)は描画テスト(MetamonWidgetTests)にもコンパイルするので、Widget 型や #Preview を置かない。
// 押した時の動きはコントロール(#1993)と同じ `OpenRecordFromControlIntent`(本体を開いて記録の入力画面を出す)。

enum WidgetActionLayout {
    static let symbol = "plus.circle"

    /// 並べられるボタンの数。入らない分は切る
    static func limit(for family: WidgetFamily) -> Int {
        switch family {
        case .systemMedium: 4
        case .accessoryRectangular: 1
        default: 2
        }
    }

    static var unselected: LocalizedStringKey {
        #if APP_STORE_REVIEW // 審査用の版は記録帳と言う(#247・docs/store/wording.md)
        "記録帳を選ぶ"
        #else
        "アプリを選ぶ"
        #endif
    }
}

struct ActionEntry: TimelineEntry {
    let date: Date
    /// 選んだアプリの今の名前(消した・未選択なら nil)
    let appName: String?
    let buttons: [ControlCatalog.RecordTarget]
    /// 押して記録した後に変わる数字(`WidgetSnapshotWriter` が書いた同じアプリの先頭 stat)
    var stat: WidgetSnapshot.Stat? = nil

    static let sample = ActionEntry(
        date: .now,
        appName: "習慣",
        buttons: [
            .init(appId: "a1", entityId: "check", appName: "習慣", entityName: "チェック", showsEntity: true),
            .init(appId: "a1", entityId: "water", appName: "習慣", entityName: "水を飲む", showsEntity: true),
        ],
        stat: .init(key: "check|count||今日の回数", label: "今日の回数", value: "3 件")
    )
}

struct ActionButtonsView: View {
    @Environment(\.widgetFamily) private var family
    let entry: ActionEntry

    private var isAccessory: Bool { family == .accessoryRectangular }

    var body: some View {
        if entry.buttons.isEmpty {
            UnselectedAppView(compact: isAccessory)
        } else if isAccessory {
            AccessoryActionView(entry: entry)
        } else {
            HomeActionView(entry: entry)
        }
    }
}

/// 未選択・消したアプリ: ウィジェットの設定で選んでもらう
struct UnselectedAppView: View {
    let compact: Bool

    var body: some View {
        VStack(spacing: 6) {
            Image(systemName: WidgetActionLayout.symbol)
                .font(compact ? .caption : .title3)
                .foregroundStyle(.secondary)
            Text(WidgetActionLayout.unselected)
                .font(compact ? .caption2 : .caption)
                .foregroundStyle(.secondary)
                .multilineTextAlignment(.center)
                .lineLimit(2)
                .minimumScaleFactor(0.7)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }
}

/// ホーム画面(small / medium): アプリ名と数字の下にボタンを並べる
struct HomeActionView: View {
    let entry: ActionEntry

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            if let stat = entry.stat {
                HStack(alignment: .firstTextBaseline, spacing: 6) {
                    Text(stat.label)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                    Spacer(minLength: 2)
                    StatValueText(value: stat.value, size: 20)
                }
            }
            VStack(spacing: 6) {
                ForEach(entry.buttons) { target in
                    ActionButton(target: target)
                }
            }
            Spacer(minLength: 0)
            if let appName = entry.appName {
                AppFooter(appName: appName, updatedAt: nil)
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
    }
}

/// ロック画面(accessoryRectangular): 単色なので文字の太さで区別する
struct AccessoryActionView: View {
    let entry: ActionEntry

    var body: some View {
        VStack(alignment: .leading, spacing: 1) {
            if let appName = entry.appName {
                Text(appName)
                    .font(.caption2.weight(.semibold))
                    .widgetAccentable()
                    .lineLimit(1)
            }
            ForEach(entry.buttons) { target in
                ActionButton(target: target, compact: true)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}

struct ActionButton: View {
    let target: ControlCatalog.RecordTarget
    var compact = false

    var body: some View {
        Button(intent: OpenRecordFromControlIntent(appId: target.appId, entityId: target.entityId)) {
            HStack(spacing: 4) {
                Image(systemName: WidgetActionLayout.symbol)
                    .font(.caption)
                Text(target.entityName) // 記録先の名前はデータなので訳さない
                    .font(compact ? .caption2.weight(.semibold) : .caption.weight(.semibold))
                    .lineLimit(1)
                    .minimumScaleFactor(0.7)
                Spacer(minLength: 0)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(.horizontal, compact ? 0 : 8)
            .padding(.vertical, compact ? 0 : 6)
            .background {
                if !compact {
                    RoundedRectangle(cornerRadius: 10, style: .continuous)
                        .fill(WidgetTheme.brandA.opacity(0.18))
                }
            }
        }
        .buttonStyle(.plain)
    }
}
