import Foundation
import MetamonServices

/// 「次の通知」の一覧(#228・#1325)。iOS の設定シートと Mac のツールバーのポップオーバーで、
/// 出す件数・時刻の書き方・「予約中の通知はありません」を出す条件を揃える。
struct UpcomingNotificationList: Equatable {
    /// 先頭から何件まで出すか
    static let limit = 5

    struct Row: Equatable, Identifiable {
        let id: Int
        let title: String
        let body: String
        /// `NotificationCopy.when`(今日なら時刻だけ)
        let when: String
    }

    let rows: [Row]
    /// 予約も見張っている場所も無い(場所の通知が位置情報の許可待ちの時は、その案内を出すので出さない)
    let isEmpty: Bool

    /// `places` / `locationOff` は場所の通知(iOS だけ)。Mac は場所を見張らないので既定値のまま。
    init(_ upcoming: [UpcomingNotification], places: Int = 0, locationOff: Bool = false, now: Date = .now) {
        rows = upcoming.prefix(Self.limit).enumerated().map { index, n in
            Row(id: index, title: n.title, body: n.body, when: NotificationCopy.when(n.date, now: now))
        }
        isEmpty = upcoming.isEmpty && places == 0 && !locationOff
    }
}
