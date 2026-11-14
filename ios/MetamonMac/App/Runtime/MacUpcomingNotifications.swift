import SwiftUI

/// このアプリの「次の通知」(#1325)。iOS はアプリごとの設定シートの節(#228)、Mac の設定ウインドウ(⌘,)は
/// アプリに紐づかないので、中央のアプリのツールバーのベルからポップオーバーで出す(「アプリ」メニューからも開く)。
/// 並び・件数・空の判定は iOS と同じ `UpcomingNotificationList`。
struct MacUpcomingNotificationsToolbar: ToolbarContent {
    @Bindable var notifications: MacRuntimeNotifications

    var body: some ToolbarContent {
        ToolbarItem(placement: .secondaryAction) {
            Button { notifications.showsUpcoming.toggle() } label: {
                Label("次の通知", systemImage: "bell")
            }
            .help("このアプリの次の通知を見る")
            .accessibilityIdentifier("upcomingNotifications.button")
            .popover(isPresented: $notifications.showsUpcoming, arrowEdge: .bottom) {
                MacUpcomingNotificationsView(notifications: notifications)
            }
        }
    }
}

struct MacUpcomingNotificationsView: View {
    let notifications: MacRuntimeNotifications

    private var list: UpcomingNotificationList { UpcomingNotificationList(notifications.upcoming) }

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            Text("次の通知")
                .font(.headline)
                .accessibilityAddTraits(.isHeader)
            if notifications.denied {
                Label("通知がオフのため、このアプリの通知は届きません", systemImage: "bell.slash")
                    .foregroundStyle(Theme.Colors.warning)
            }
            if list.isEmpty {
                Text("予約中の通知はありません")
                    .foregroundStyle(.secondary)
            } else {
                VStack(alignment: .leading, spacing: 8) {
                    ForEach(list.rows) { row($0) }
                }
            }
            Divider()
            Text("アプリを閉じていても、この時刻に届きます。")
                .font(.caption)
                .foregroundStyle(.secondary)
        }
        .padding(14)
        .frame(width: 320, alignment: .leading)
        .accessibilityIdentifier("upcomingNotifications")
        .task { await notifications.refresh() } // 開いた時に予約し直す(届いた分を外す・iOS の設定シートと同じ)
    }

    private func row(_ n: UpcomingNotificationList.Row) -> some View {
        HStack(alignment: .firstTextBaseline, spacing: 12) {
            VStack(alignment: .leading, spacing: 2) {
                Text(n.title)
                if !n.body.isEmpty {
                    Text(n.body).font(.caption).foregroundStyle(.secondary).lineLimit(1)
                }
            }
            Spacer(minLength: 8)
            Text(n.when).monospacedDigit()
        }
        .accessibilityElement(children: .combine) // 1 行を「題・本文・時刻」で 1 回に読む
    }
}
