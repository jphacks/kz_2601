import Foundation
import UserNotifications
import MetamonDSL
#if os(iOS)
import CoreLocation
#endif

/// `UNUserNotificationCenter` によるローカル通知(#228)。
/// - アプリごとの予約(`PlannedNotification`)を UserDefaults に持ち、別アプリに切り替えても前のアプリの通知を消さない。
/// - 予約し直す時は全アプリ分から近い順に `NotificationPlanner.limit` 件だけ iOS に渡す(64 件を超えた分が黙って捨てられないように)。
/// - 識別子は `dsl.<appId>.…`。それ以外(タイマー `timer.` 等)には触らない。
/// - 場所の通知(`near`)は全アプリ分から `NotificationPlanner.regionLimit` か所(近い順・新しい順)を `UNLocationNotificationTrigger` で見張る。
///   位置情報(使用中のみ)の許可は、場所を保存したアプリの予約の時に未決定なら 1 回だけ聞く。判定は iOS が端末の中で行う。
/// - 通知の `filterCriteria` は生成アプリの id。集中モードのフィルタ(`TailorFocusFilter`)の「通知を止める」で、
///   選んでいないアプリの通知を OS が出さない(予約は今のまま・#1992)。
/// - `alarm` の通知は、`alarms` があり許可されていれば止めるまで鳴るアラーム(AlarmKit)で届け、普通の通知は出さない。
///   無い(iOS 26.1 未満・審査用の版)・許可が無い・予約に失敗した時は普通の通知で届ける。
public struct UserNotificationScheduler: NotificationScheduler {
    private let alarmSync: AlarmSync?
    private let center: any NotificationCenterClient
    private let registry: NotificationRegistry
    private let regionStore: RegionNotificationStore
    private let location: RegionAccess
    /// 予約し直しを 1 本ずつに。重なると、片方が読んだ古い予約一覧で「見張ったのに予約に無い = 届いた」と誤る(1 回きりの場所の通知)。
    private let queue: AsyncSerialQueue

    public init(alarms: (any AlarmScheduling)? = nil) {
        self.init(alarms: alarms, center: SystemNotificationCenter())
    }

    init(alarms: (any AlarmScheduling)? = nil, center: any NotificationCenterClient,
         registry: NotificationRegistry = .shared, regionStore: RegionNotificationStore = .shared,
         location: RegionAccess = .system, queue: AsyncSerialQueue = .notifications) {
        alarmSync = alarms.map(AlarmSync.init)
        self.center = center
        self.registry = registry
        self.regionStore = regionStore
        self.location = location
        self.queue = queue
    }

    public func sync(appId: String, specs: [NotificationSpec], records: [String: [Record]]) async -> NotificationSyncResult {
        await registry.set(appId, NotificationPlanner.plan(appId: appId, specs: specs, records: records))
        return await apply(appId: appId)
    }

    public func schedule(_ spec: NotificationSpec, appId: String) async throws -> Date? {
        guard let p = NotificationPlanner.action(appId: appId, spec: spec) else { return nil }
        await registry.upsertAction(appId, p)
        _ = await apply(appId: appId)
        return p.nextFire(after: Date())
    }

    public func cancel(id: String, appId: String) async {
        await registry.remove(appId: appId, specId: id)
        _ = await apply(appId: appId)
    }

    public func post(_ spec: NotificationSpec, appId: String) async -> Bool {
        guard await requestPermissionIfNeeded() else { return false }
        let content = UNMutableNotificationContent()
        content.title = spec.title
        content.body = spec.body
        content.sound = .default
        content.filterCriteria = appId // 集中モードのフィルタで絞る(#1992)
        // 予約(apply)が消す "dsl." とは別の名前空間。1 秒後に出るので登録簿には残さない
        let request = UNNotificationRequest(
            identifier: ConditionNotifications.identifier(appId: appId, specId: spec.id), content: content,
            trigger: UNTimeIntervalNotificationTrigger(timeInterval: 1, repeats: false))
        do {
            try await center.add(request)
            return true
        } catch {
            print("[Metamon] condition notification failed id=\(spec.id): \(error)")
            return false
        }
    }

    public func removeApp(_ appId: String) async {
        if let alarmSync {
            await alarmSync.removeApp(await registry.planned(appId), now: Date())
        }
        await registry.removeApp(appId)
        await ConditionNotificationStore.shared.removeApp(appId)
        await regionStore.removeApp(appId)
        _ = await apply(appId: appId)
    }

    public func removeAll() async {
        await alarmSync?.removeAll()
        await registry.removeAll()
        await ConditionNotificationStore.shared.removeAll()
        await regionStore.removeAll()
        let center = center
        await queue.run {
            let ids = await center.pendingIdentifiers()
                .filter { $0.hasPrefix(NotificationPlanner.prefix) || $0.hasPrefix(ConditionNotifications.prefix) }
            await center.removePending(ids)
        }
    }

    public func refresh(appId: String) async -> NotificationSyncResult { await apply(appId: appId) }

    /// 全アプリ分を近い順に上限まで予約し直す。結果の upcoming は `appId` の分だけ。直列(`queue`)。
    private func apply(appId: String) async -> NotificationSyncResult {
        await queue.run { await self.applyNow(appId: appId) }
    }

    private func applyNow(appId: String) async -> NotificationSyncResult {
        let now = Date()
        let all = await registry.pruned(now: now)
        let owners = await registry.owners()
        let mine = NotificationPlanner.prefix + appId + "."
        let pending = await center.pendingIdentifiers().filter { $0.hasPrefix(NotificationPlanner.prefix) }
        let place = await places(all, appId: appId, pending: Set(pending))
        let chosen = NotificationPlanner.select(all, now: now, limit: NotificationPlanner.limit - place.regions.count)
        let asAlarm = await alarmSync?.apply(chosen.map(\.0), now: now) ?? []
        let notify = chosen.map(\.0).filter { !asAlarm.contains($0.identifier) } + place.regions
        let want = Set(notify.map(\.identifier))
        await center.removePending(pending.filter { !want.contains($0) })
        var denied = false
        if !notify.isEmpty {
            denied = await !requestPermissionIfNeeded()
        }
        var armed: [PlannedNotification] = []
        for p in notify {
            do {
                try await center.add(Self.request(p, appId: owners[p.identifier]))
                if p.isRegion { armed.append(p) }
            } catch {
                print("[Metamon] notification add failed id=\(p.identifier): \(error)")
            }
        }
        await regionStore.setArmed(armed)
        let upcoming = chosen.filter { $0.0.identifier.hasPrefix(mine) }.map { UpcomingNotification(title: $0.0.title, body: $0.0.body, date: $0.1) }
        let myPlaces = place.regions.filter { $0.identifier.hasPrefix(mine) }.count
        print("[Metamon] notifications scheduled: total=\(chosen.count) alarms=\(asAlarm.count) regions=\(armed.count) app=\(upcoming.count) places=\(myPlaces) next=\(upcoming.first.map { "\($0.date)" } ?? "-") denied=\(denied) locationOff=\(place.locationOff)")
        return NotificationSyncResult(denied: denied, upcoming: upcoming, places: myPlaces, locationOff: place.locationOff)
    }

    /// 見張る場所の通知。位置情報が許可されていなければ見張らない(`locationOff` = このアプリに場所があるのに届かない)。
    /// 未決定なら、このアプリに場所がある時だけ許可を聞く(場所を保存した直後・そのアプリを開いた時 = 使う場面)。
    private func places(_ all: [PlannedNotification], appId: String, pending: Set<String>) async -> (regions: [PlannedNotification], locationOff: Bool) {
        let known = all.filter(\.isRegion)
        guard !known.isEmpty else {
            await regionStore.setArmed([])
            return ([], false)
        }
        let hasMine = known.contains { $0.identifier.hasPrefix(NotificationPlanner.prefix + appId + ".") }
        let allowed = hasMine ? await location.authorize() : await location.isAuthorized()
        guard allowed else {
            await regionStore.setArmed([])
            return ([], hasMine)
        }
        let candidates = await regionStore.toSchedule(known, known: known, pending: pending)
        return (NotificationPlanner.selectRegions(candidates, here: await location.lastKnown()), false)
    }

    /// 許可されていれば true。未決定ならここでダイアログを出す。
    public func requestPermissionIfNeeded() async -> Bool {
        switch await center.authorizationStatus() {
        case .notDetermined:
            return await center.requestAuthorization()
        case .denied:
            return false
        default:
            return true
        }
    }

    /// `appId` = 予約した生成アプリ。`filterCriteria` に入れ、集中モードのフィルタで OS が絞る(#1992)。
    static func request(_ p: PlannedNotification, appId: String? = nil, calendar: Calendar = .current) -> UNNotificationRequest {
        let content = UNMutableNotificationContent()
        content.title = p.title
        content.body = p.body
        content.sound = .default
        content.filterCriteria = appId
        let trigger: UNNotificationTrigger
        switch p.trigger {
        case .at(let d):
            trigger = UNCalendarNotificationTrigger(dateMatching: calendar.dateComponents([.year, .month, .day, .hour, .minute, .second], from: d), repeats: false)
        case .daily(let h, let m):
            trigger = UNCalendarNotificationTrigger(dateMatching: DateComponents(hour: h, minute: m), repeats: true)
        case .weekly(let w, let h, let m):
            trigger = UNCalendarNotificationTrigger(dateMatching: DateComponents(hour: h, minute: m, weekday: w), repeats: true)
        case .monthly(let d, let h, let m):
            trigger = UNCalendarNotificationTrigger(dateMatching: DateComponents(day: d, hour: h, minute: m), repeats: true)
        case .yearly(let mo, let d, let h, let m):
            trigger = UNCalendarNotificationTrigger(dateMatching: DateComponents(month: mo, day: d, hour: h, minute: m), repeats: true)
        case .region(let lat, let lng, let radius, let repeats):
            #if os(iOS)
            let region = CLCircularRegion(center: CLLocationCoordinate2D(latitude: lat, longitude: lng), radius: radius, identifier: p.identifier)
            region.notifyOnEntry = true
            region.notifyOnExit = false
            trigger = UNLocationNotificationTrigger(region: region, repeats: repeats)
            #else
            _ = (lat, lng, radius, repeats) // macOS(パッケージのテスト)は場所の通知を予約しない(`places` が空)
            trigger = UNTimeIntervalNotificationTrigger(timeInterval: 60, repeats: false)
            #endif
        }
        return UNNotificationRequest(identifier: p.identifier, content: content, trigger: trigger)
    }

    // 旧 API 互換(テスト)
    public static func parseTime(_ t: String?) -> (Int, Int)? { NotificationPlanner.parseTime(t) }
    public static func weekdayNumber(_ w: String?) -> Int { NotificationPlanner.weekdayNumber(w) }
}

/// アプリごとの予約の保存先(UserDefaults・直列化のため actor)。キー: appId(DSL 分)/ appId + "#action"(ボタン分)。
actor NotificationRegistry {
    static let shared = NotificationRegistry()
    private static let key = "metamon.notifications.planned.v1"
    private let defaults: UserDefaults
    private var table: [String: [PlannedNotification]]

    init(defaults: UserDefaults = .standard) {
        self.defaults = defaults
        if let data = defaults.data(forKey: Self.key), let t = try? JSONDecoder().decode([String: [PlannedNotification]].self, from: data) {
            table = t
        } else {
            table = [:]
        }
    }

    func set(_ appId: String, _ planned: [PlannedNotification]) {
        table[appId] = planned
        save()
    }

    func upsertAction(_ appId: String, _ p: PlannedNotification) {
        var list = table[appId + "#action"] ?? []
        list.removeAll { $0.identifier == p.identifier }
        list.append(p)
        table[appId + "#action"] = list
        save()
    }

    func remove(appId: String, specId: String) {
        let ids = [NotificationPlanner.identifier(appId: appId, specId: specId), NotificationPlanner.actionIdentifier(appId: appId, specId: specId)]
        let recordPrefix = ids[0] + "."
        for k in [appId, appId + "#action"] {
            table[k]?.removeAll { ids.contains($0.identifier) || $0.identifier.hasPrefix(recordPrefix) }
        }
        save()
    }

    /// そのアプリの予約(DSL 分とボタン分)。
    func planned(_ appId: String) -> [PlannedNotification] {
        (table[appId] ?? []) + (table[appId + "#action"] ?? [])
    }

    /// 予約の識別子 → 生成アプリの id(通知の `filterCriteria`・#1992)。
    func owners() -> [String: String] {
        var map: [String: String] = [:]
        for (key, list) in table {
            let appId = key.hasSuffix("#action") ? String(key.dropLast("#action".count)) : key
            for p in list { map[p.identifier] = appId }
        }
        return map
    }

    func removeApp(_ appId: String) {
        table[appId] = nil
        table[appId + "#action"] = nil
        save()
    }

    func removeAll() {
        table = [:]
        save()
    }

    /// 過ぎた 1 回きりを捨てて、全件を返す(場所の通知は時刻が無いので残す)。
    func pruned(now: Date) -> [PlannedNotification] {
        var changed = false
        for (k, list) in table {
            let kept = list.filter { $0.isRegion || $0.nextFire(after: now) != nil }
            if kept.count != list.count { table[k] = kept; changed = true }
        }
        if changed { save() }
        return table.values.flatMap { $0 }
    }

    private func save() {
        if let data = try? JSONEncoder().encode(table) { defaults.set(data, forKey: Self.key) }
    }
}
