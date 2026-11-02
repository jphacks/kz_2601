import Foundation

/// 毎月 / 毎年くり返す日付(#266)。式の `nextOccurrence` とレコード基準の通知(`repeat: monthly|yearly`)で共通。
/// 日が無い月は月末に寄せる(1/31 の毎月 → 2/28・閏年 2/29、2/29 の毎年 → 平年 2/28)。
/// 寄せるのはその回だけで、次の回は元の日に戻る(1/31 → 2/28 → 3/31)。
public enum RepeatDate {
    /// "monthly" = 1 か月、"yearly" = 12 か月。それ以外は nil。
    public static func months(_ unit: String) -> Int? {
        switch unit {
        case "monthly": 1
        case "yearly": 12
        default: nil
        }
    }

    /// 起点から k 回目(0 = 起点)。時刻はそのまま、日は月末に寄せる(`Calendar.date(byAdding: .month)` の挙動)。
    public static func occurrence(_ anchor: Date, months: Int, index k: Int, calendar: Calendar) -> Date? {
        calendar.date(byAdding: .month, value: months * k, to: anchor)
    }

    /// `anchor` と `date` の暦の月の差(date の月 − anchor の月)。
    public static func monthsBetween(_ anchor: Date, _ date: Date, calendar: Calendar) -> Int {
        let a = calendar.dateComponents([.year, .month], from: anchor)
        let b = calendar.dateComponents([.year, .month], from: date)
        return ((b.year ?? 0) - (a.year ?? 0)) * 12 + (b.month ?? 0) - (a.month ?? 0)
    }

    /// 今日以降で最も近い回の日付(0:00)。起点が今日以降なら起点の日。日単位で比べる(時刻は落とす)。
    public static func next(_ anchor: Date, unit: String, today: Date, calendar: Calendar) -> Date? {
        guard let m = months(unit) else { return nil }
        let a = calendar.startOfDay(for: anchor)
        let t = calendar.startOfDay(for: today)
        if a >= t { return a }
        var k = max(monthsBetween(a, t, calendar: calendar) / m, 0)
        for _ in 0..<4 {
            guard let d = occurrence(a, months: m, index: k, calendar: calendar) else { return nil }
            let day = calendar.startOfDay(for: d)
            if day >= t { return day }
            k += 1
        }
        return nil
    }
}
