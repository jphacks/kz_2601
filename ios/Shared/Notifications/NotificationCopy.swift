import Foundation

/// 通知の時刻の書き方(#228)。iOS と Mac で同じ(#965)。
enum NotificationCopy {
    /// 今日なら「7:00」、それ以外は「10/26(日) 7:00」(端末の言語の書式)。
    static func when(_ d: Date, now: Date = .now) -> String {
        let f = DateFormatter()
        f.locale = .current
        f.setLocalizedDateFormatFromTemplate(Calendar.current.isDate(d, inSameDayAs: now) ? "Hm" : "MdEEEHm")
        return f.string(from: d)
    }
}

/// 押された通知がどのアプリのものか(#965)。識別子の形は
/// `dsl.<appId>.…`(予約)・`when.<appId>.…`(条件)・`timer.<appId>/…`(タイマー)。
/// appId に「.」が入っても誤らないよう、一覧にある id のうち一番長く合うものを選ぶ。
enum NotificationAppRoute {
    static func appId(for identifier: String, among ids: [String]) -> String? {
        let scopes: [(prefix: String, separator: String)] = [("dsl.", "."), ("when.", "."), ("timer.", "/")]
        guard let scope = scopes.first(where: { identifier.hasPrefix($0.prefix) }) else { return nil }
        let rest = identifier.dropFirst(scope.prefix.count)
        return ids.filter { !$0.isEmpty && rest.hasPrefix($0 + scope.separator) }.max { $0.count < $1.count }
    }
}
