import CryptoKit
import Foundation

/// アラームの時刻(AlarmKit の `Alarm.Schedule` の写し。iOS 26 の型に依らずテストできるように)。
public enum AlarmTrigger: Hashable, Sendable {
    /// その日時に 1 回
    case fixed(Date)
    /// 毎週その曜日のその時刻(weekdays は 1 = 日 〜 7 = 土。毎日は 7 つ全部)
    case relative(hour: Int, minute: Int, weekdays: [Int])
}

/// AlarmKit に予約するアラーム 1 件。`id` は中身から決まる(中身が変われば別の id になり、古い方は消える)。
public struct AlarmRequest: Hashable, Sendable {
    public var id: UUID
    /// 元の通知の識別子(`dsl.<appId>.…`)
    public var identifier: String
    public var title: String
    public var trigger: AlarmTrigger
}

/// 予約済みのアラーム。`alerting` は今鳴っているもの(予約し直しでは止めない)。
public struct ScheduledAlarm: Hashable, Sendable {
    public var id: UUID
    public var alerting: Bool
    public init(id: UUID, alerting: Bool) {
        self.id = id
        self.alerting = alerting
    }
}

/// 止めるまで鳴るアラーム(AlarmKit・iOS 26.1 以降)。実装はアプリ本体(`AlarmKitAlarms`)、テストでは差し替える。
/// 無い(古い iOS・審査用の版)時はアラームの通知も普通のローカル通知で届ける。
public protocol AlarmScheduling: Sendable {
    /// 許可されていれば true。未決定ならここで許可を求める。
    func authorize() async -> Bool
    /// このアプリが予約したアラーム(AlarmKit はアプリごとに分かれている)。
    func scheduled() async -> [ScheduledAlarm]
    func schedule(_ request: AlarmRequest) async throws
    func cancel(_ id: UUID) async
}

/// 予約する通知 → アラームの組み立て(純粋関数)。
public enum AlarmPlanner {
    public static let everyDay = Array(1...7)

    /// 毎月 / 毎年は AlarmKit にくり返しが無いので次の 1 回だけ(アプリを開いた時の予約し直しで次へ延ばす)。
    public static func trigger(for p: PlannedNotification, now: Date, calendar: Calendar = .current) -> AlarmTrigger? {
        switch p.trigger {
        case .at(let d):
            return d > now ? .fixed(d) : nil
        case .daily(let h, let m):
            return .relative(hour: h, minute: m, weekdays: everyDay)
        case .weekly(let w, let h, let m):
            return .relative(hour: h, minute: m, weekdays: [w])
        case .monthly, .yearly:
            return p.nextFire(after: now, calendar: calendar).map { .fixed($0) }
        }
    }

    /// アラームの画面の見出し: 通知の見出しと本文の 1 行目(レコードのラベル)。
    public static func title(_ p: PlannedNotification) -> String {
        let label = p.body.split(separator: "\n").first.map(String.init) ?? ""
        if label.isEmpty || label == p.title { return p.title }
        return p.title.isEmpty ? label : p.title + " " + label
    }

    public static func requests(_ planned: [PlannedNotification], now: Date, calendar: Calendar = .current) -> [AlarmRequest] {
        planned.filter(\.isAlarm).compactMap { p in
            guard let t = trigger(for: p, now: now, calendar: calendar) else { return nil }
            let title = title(p)
            return AlarmRequest(id: id(identifier: p.identifier, title: title, trigger: t), identifier: p.identifier, title: title, trigger: t)
        }
    }

    /// 中身(識別子・見出し・時刻)から決まる UUID。
    static func id(identifier: String, title: String, trigger: AlarmTrigger) -> UUID {
        let when: String
        switch trigger {
        case .fixed(let d): when = "at:\(Int(d.timeIntervalSince1970))"
        case .relative(let h, let m, let ws): when = "rel:\(h):\(m):" + ws.map(String.init).joined(separator: ",")
        }
        var bytes = Array(SHA256.hash(data: Data([identifier, title, when].joined(separator: "\u{1F}").utf8)).prefix(16))
        bytes[6] = (bytes[6] & 0x0F) | 0x50 // version 5 相当(名前から作った UUID)
        bytes[8] = (bytes[8] & 0x3F) | 0x80
        return UUID(uuid: (bytes[0], bytes[1], bytes[2], bytes[3], bytes[4], bytes[5], bytes[6], bytes[7],
                           bytes[8], bytes[9], bytes[10], bytes[11], bytes[12], bytes[13], bytes[14], bytes[15]))
    }
}

/// 通知の予約し直し(`UserNotificationScheduler.apply`)のうちアラームの分。
struct AlarmSync: Sendable {
    let alarms: any AlarmScheduling

    /// 選ばれた予約のうちアラームの分を AlarmKit に合わせる。戻り値はアラームで届ける識別子(普通の通知からは外す)。
    /// 許可が無い・予約に失敗した分は返さない(普通の通知で届く)。要らなくなったアラームは消すが、鳴っている最中のものは止めない。
    func apply(_ chosen: [PlannedNotification], now: Date, calendar: Calendar = .current) async -> Set<String> {
        let wanted = chosen.filter(\.isAlarm)
        let requests = wanted.isEmpty ? [] : (await alarms.authorize() ? AlarmPlanner.requests(wanted, now: now, calendar: calendar) : [])
        let current = await alarms.scheduled()
        let keep = Set(requests.map(\.id))
        for a in current where !keep.contains(a.id) && !a.alerting {
            await alarms.cancel(a.id)
        }
        let have = Set(current.map(\.id))
        var handled = Set<String>()
        for r in requests {
            if have.contains(r.id) {
                handled.insert(r.identifier)
                continue
            }
            do {
                try await alarms.schedule(r)
                handled.insert(r.identifier)
            } catch {
                print("[Metamon] alarm schedule failed id=\(r.identifier): \(error)") // 上限など。普通の通知で届ける
            }
        }
        return handled
    }

    /// アプリ削除: そのアプリのアラームを鳴っている最中でも消す。
    func removeApp(_ planned: [PlannedNotification], now: Date) async {
        for r in AlarmPlanner.requests(planned, now: now) {
            await alarms.cancel(r.id)
        }
    }

    /// サインアウト・アカウント削除: このアプリのアラームを全部消す。
    func removeAll() async {
        for a in await alarms.scheduled() {
            await alarms.cancel(a.id)
        }
    }
}
