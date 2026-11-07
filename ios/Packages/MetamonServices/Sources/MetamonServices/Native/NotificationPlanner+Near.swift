import Foundation
import MetamonDSL

/// 場所に着いたら知らせる(`near`)と時刻のずらし(`jitterMinutes`)。純粋関数・ユニットテスト対象。
extension NotificationPlanner {
    /// iOS が 1 アプリで見張れる領域は約 20。位置を使う他の機能の分を残して、全アプリ合わせて 15 か所。
    public static let regionLimit = 15
    /// ずらす通知はくり返しの予約で書けない(回ごとに時刻が違う)ので、先まで具体的な日時で予約する回数(毎日 7・毎週 4・毎月 3・毎年 2)。
    /// 残りはアプリを開いた時の予約し直しで先へ延ばす。上限の 60 件は select が近い順に守る。
    static let jitterDailyAhead = 7

    /// レコード 1 件 → 場所の通知(場所が空・読めないレコードは nil)。
    static func planNear(appId: String, spec: NotificationSpec, near: NotificationSpec.Near, record: Record) -> PlannedNotification? {
        guard let c = Track.coordinate(record[near.locationField]) else { return nil }
        let saved = (record.updatedAt ?? record.createdAt).flatMap { FieldCoding.date(from: .string($0)) }
        return PlannedNotification(
            identifier: identifier(appId: appId, specId: spec.id, recordId: record.id), title: spec.title, body: spec.body,
            trigger: .region(latitude: c.lat, longitude: c.lng, radius: near.radius, repeats: spec.repeat != .once),
            savedAt: saved)
    }

    /// 見張る場所を全アプリ分から `limit` か所: 現在地が分かれば近い順、分からなければ新しく保存した順。
    public static func selectRegions(_ all: [PlannedNotification], here: (lat: Double, lng: Double)? = nil, limit: Int = regionLimit) -> [PlannedNotification] {
        var seen = Set<String>()
        let regions = all.filter { $0.isRegion && seen.insert($0.identifier).inserted }
        let sorted: [PlannedNotification]
        if let here {
            sorted = regions.sorted {
                let (a, b) = (distance(here, $0), distance(here, $1))
                return a != b ? a < b : $0.identifier < $1.identifier
            }
        } else {
            sorted = regions.sorted {
                let (a, b) = ($0.savedAt ?? .distantPast, $1.savedAt ?? .distantPast)
                return a != b ? a > b : $0.identifier < $1.identifier
            }
        }
        return Array(sorted.prefix(limit))
    }

    /// 2 点間のおおよその距離(m・haversine)。
    static func distance(_ here: (lat: Double, lng: Double), _ p: PlannedNotification) -> Double {
        guard case .region(let lat, let lng, _, _) = p.trigger else { return .infinity }
        let r = 6_371_000.0, rad = Double.pi / 180
        let dLat = (lat - here.lat) * rad, dLng = (lng - here.lng) * rad
        let a = sin(dLat / 2) * sin(dLat / 2) + cos(here.lat * rad) * cos(lat * rad) * sin(dLng / 2) * sin(dLng / 2)
        return 2 * r * asin(min(1, sqrt(a)))
    }

    /// 時刻の通知を回ごとに ±`minutes` 分ずらす。ずらし幅は識別子と回の時刻から決まる(予約し直しても同じ回は同じ時刻)。
    /// くり返しは近い回から具体的な日時に展開する(識別子 "<id>.j<回の分>"・取消は "<spec id>." の前方一致で一緒に消える)。
    static func jittered(_ p: PlannedNotification, minutes: Int?, now: Date, calendar: Calendar) -> [PlannedNotification] {
        guard let minutes, minutes > 0, !p.isRegion else { return [p] }
        let span = min(minutes, 60)
        func shifted(_ occurrence: Date) -> Date {
            occurrence.addingTimeInterval(TimeInterval(jitterOffset(p.identifier, occurrence, span) * 60))
        }
        let ahead: Int
        switch p.trigger {
        case .at(let d):
            let fire = shifted(d)
            guard fire > now else { return [] }
            var one = p
            one.trigger = .at(fire)
            return [one]
        case .daily: ahead = jitterDailyAhead
        case .weekly: ahead = 4
        case .monthly: ahead = monthlyAhead
        case .yearly: ahead = yearlyAhead
        case .region: return [p]
        }
        // 少し前の回も見る(早めにずれた回がまだ先かもしれない)
        var cursor = now.addingTimeInterval(TimeInterval(-span * 60))
        var out: [PlannedNotification] = []
        for _ in 0..<(ahead + 1) {
            guard let occurrence = p.nextFire(after: cursor, calendar: calendar) else { break }
            cursor = occurrence
            let fire = shifted(occurrence)
            guard fire > now else { continue }
            var one = p
            one.identifier = p.identifier + ".j\(Int(occurrence.timeIntervalSince1970 / 60))"
            one.trigger = .at(fire)
            out.append(one)
            if out.count == ahead { break }
        }
        return out
    }

    /// -span...span 分。FNV-1a(起動ごとに変わる `Hasher` は使わない)。
    static func jitterOffset(_ identifier: String, _ occurrence: Date, _ span: Int) -> Int {
        var h: UInt64 = 0xcbf2_9ce4_8422_2325
        for b in "\(identifier)@\(Int(occurrence.timeIntervalSince1970))".utf8 {
            h = (h ^ UInt64(b)) &* 0x100_0000_01b3
        }
        return Int(h % UInt64(2 * span + 1)) - span
    }
}
