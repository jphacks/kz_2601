// 審査用ビルド(APP_STORE_REVIEW・#245)ではバイナリから外す(entitlement も Metamon-Review.entitlements に入れない)。
#if !APP_STORE_REVIEW
import Foundation
#if os(iOS)
import HomeKit
#endif

/// HomeKit の家・部屋・機器と今の状態を読む口(#2245・親 #1707)。部品・アクションはまだ無い(決定待ち)。
/// `DeviceCalendarSource` と同じ形: protocol + 実体(`LiveDeviceHome`)+ 偽物(`FakeDeviceHome`)。
/// 機器を操作する書き込みは持たない(読むだけ)。家・部屋・機器の名前はログ・クラッシュ報告・サーバーに出さない(`description` も伏せる)。
public protocol DeviceHomeSource: Sendable {
    /// 許可の状態。`requesting: true` で未決定なら OS の許可ダイアログを出す(`HMHomeManager` を作る)。
    func authorization(requesting: Bool) async -> DeviceHomeAccess
    /// 家の一覧(名前順・代表の家が先)。許可が無ければ `DeviceHomeError.notAuthorized`。
    func homes() async throws -> [DeviceHomeInfo]
    /// 部屋(名前順)。`homeId` が nil なら代表の家。
    func rooms(homeId: String?) async throws -> [DeviceRoom]
    /// 機器(部屋順・名前順)。`homeId` が nil なら代表の家、`roomId` が nil なら家の全部屋。
    func accessories(homeId: String?, roomId: String?) async throws -> [DeviceAccessory]
    /// 家・部屋・機器の構成が変わったことを知らせる。中身は渡さない。
    func changes() -> AsyncStream<Void>
}

/// HomeKit の許可状態を自前の値にしたもの(`HMHomeManagerAuthorizationStatus` を写す)。
public enum DeviceHomeAccess: String, Sendable, Equatable {
    case notDetermined
    case restricted
    case authorized

    public var canRead: Bool { self == .authorized }
}

public enum DeviceHomeError: LocalizedError, Equatable {
    /// 許可が無い(未決定・制限)
    case notAuthorized(DeviceHomeAccess)
    /// この端末では使えない(iOS 以外・HomeKit を持たない端末)
    case unavailable
    /// 家が 1 つも無い(ホーム App で作っていない)
    case noHome

    public var errorDescription: String? {
        switch self {
        case .notAuthorized: L("ホームへのアクセスが許可されていません")
        case .unavailable: L("この端末ではホームを使えません")
        case .noHome: L("ホーム App に家がありません")
        }
    }
}

/// 1 回で返す件数の上限。超えた分は切る(エラーにしない)。
public enum DeviceHomeLimits {
    public static let maxItems = 200
}

public struct DeviceHomeInfo: Sendable, Equatable, Identifiable, CustomStringConvertible, CustomDebugStringConvertible {
    public var id: String
    public var name: String
    /// 代表の家(`HMHomeManager.primaryHome`)
    public var isPrimary: Bool

    public init(id: String, name: String, isPrimary: Bool) {
        self.id = id
        self.name = name
        self.isPrimary = isPrimary
    }

    public var description: String { "DeviceHomeInfo(id: \(id), primary: \(isPrimary))" }
    public var debugDescription: String { description }
}

public struct DeviceRoom: Sendable, Equatable, Identifiable, CustomStringConvertible, CustomDebugStringConvertible {
    public var id: String
    public var name: String
    public var homeId: String

    public init(id: String, name: String, homeId: String) {
        self.id = id
        self.name = name
        self.homeId = homeId
    }

    public var description: String { "DeviceRoom(id: \(id))" }
    public var debugDescription: String { description }
}

/// 機器の種類。HomeKit の `HMAccessoryCategoryType` を、文字にして見せられる範囲にまとめたもの。
public enum DeviceAccessoryKind: String, Sendable, CaseIterable {
    case light
    case outlet
    case powerSwitch
    case thermostat
    case airConditioner
    case fan
    case airPurifier
    case humidifier
    case sensor
    case lock
    case windowCovering
    case camera
    case speaker
    case bridge
    case other

    /// 画面に出す名前(`LanguagePreference` の言語)。
    public var label: String {
        switch self {
        case .light: L("照明")
        case .outlet: L("コンセント")
        case .powerSwitch: L("スイッチ")
        case .thermostat: L("サーモスタット")
        case .airConditioner: L("エアコン")
        case .fan: L("扇風機")
        case .airPurifier: L("空気清浄機")
        case .humidifier: L("加湿器")
        case .sensor: L("センサー")
        case .lock: L("鍵")
        case .windowCovering: L("カーテン")
        case .camera: L("カメラ")
        case .speaker: L("スピーカー")
        case .bridge: L("ハブ")
        case .other: L("その他の機器")
        }
    }
}

/// よく使う特性。値の単位は `DeviceHomeReading`。
public enum DeviceHomeTrait: String, Sendable, CaseIterable {
    /// 電源。値は 0(オフ)/ 1(オン)
    case power
    /// 明るさ。0...100 %
    case brightness
    /// 温度。°C
    case temperature
    /// 湿度。0...100 %
    case humidity

    public var label: String {
        switch self {
        case .power: L("電源")
        case .brightness: L("明るさ")
        case .temperature: L("温度")
        case .humidity: L("湿度")
        }
    }
}

public struct DeviceHomeReading: Sendable, Equatable, Identifiable {
    public var trait: DeviceHomeTrait
    public var value: Double
    public var id: String { trait.rawValue }

    public init(trait: DeviceHomeTrait, value: Double) {
        self.trait = trait
        self.value = value
    }

    /// 今の値を文字にする(純関数)。電源はオン / オフ、明るさ・湿度は整数 %、温度は小数 1 桁の °C。
    public var text: String {
        switch trait {
        case .power: value >= 0.5 ? L("オン") : L("オフ")
        case .brightness, .humidity: "\(Int(value.rounded()))%"
        case .temperature: String(format: "%.1f°C", value)
        }
    }
}

public struct DeviceAccessory: Sendable, Equatable, Identifiable, CustomStringConvertible, CustomDebugStringConvertible {
    public var id: String
    public var name: String
    public var kind: DeviceAccessoryKind
    public var homeId: String
    public var roomId: String?
    public var roomName: String?
    /// 今つながっているか(`HMAccessory.isReachable`)。false の時 `readings` は最後に読めた値か空
    public var isReachable: Bool
    /// よく使う特性の今の値(特性の並び順)
    public var readings: [DeviceHomeReading]

    public init(id: String, name: String, kind: DeviceAccessoryKind, homeId: String,
                roomId: String?, roomName: String?, isReachable: Bool, readings: [DeviceHomeReading]) {
        self.id = id
        self.name = name
        self.kind = kind
        self.homeId = homeId
        self.roomId = roomId
        self.roomName = roomName
        self.isReachable = isReachable
        self.readings = readings
    }

    /// 名前は出さない(誤ってログに入れても漏れない)
    public var description: String { "DeviceAccessory(id: \(id), kind: \(kind.rawValue), reachable: \(isReachable))" }
    public var debugDescription: String { description }
}

@MainActor
public enum DeviceHome {
    /// 読む口の実体。アプリ起動時に 1 回だけ差し替える(`METAMON_MOCK=1` は `FakeDeviceHome.sample`)。
    public static var source: any DeviceHomeSource = LiveDeviceHome()
}

// MARK: - 実体と偽物で共有する絞り込み(単体テストの対象)

enum DeviceHomeQuery {
    /// 代表の家が先・あとは名前順。
    static func homes(_ all: [DeviceHomeInfo]) -> [DeviceHomeInfo] {
        let sorted = all.sorted { a, b in
            if a.isPrimary != b.isPrimary { return a.isPrimary }
            if a.name != b.name { return a.name.localizedStandardCompare(b.name) == .orderedAscending }
            return a.id < b.id
        }
        return Array(sorted.prefix(DeviceHomeLimits.maxItems))
    }

    /// `homeId` が nil なら代表の家(無ければ最初の家)の id。家が 1 つも無ければ nil。
    static func resolvedHomeId(_ homeId: String?, in all: [DeviceHomeInfo]) -> String? {
        if let homeId { return all.contains { $0.id == homeId } ? homeId : nil }
        return homes(all).first?.id
    }

    static func rooms(_ all: [DeviceRoom], homeId: String) -> [DeviceRoom] {
        let hits = all.filter { $0.homeId == homeId }
        let sorted = hits.sorted { a, b in
            if a.name != b.name { return a.name.localizedStandardCompare(b.name) == .orderedAscending }
            return a.id < b.id
        }
        return Array(sorted.prefix(DeviceHomeLimits.maxItems))
    }

    /// 家と部屋で絞り、部屋の名前順(部屋なしは後ろ)・機器の名前順に並べて上限で切る。
    static func accessories(_ all: [DeviceAccessory], homeId: String, roomId: String?) -> [DeviceAccessory] {
        let hits = all.filter { a in
            a.homeId == homeId && (roomId.map { $0 == a.roomId } ?? true)
        }
        let sorted = hits.sorted { a, b in
            switch (a.roomName, b.roomName) {
            case let (x?, y?) where x != y: return x.localizedStandardCompare(y) == .orderedAscending
            case (.some, nil): return true
            case (nil, .some): return false
            default: break
            }
            if a.name != b.name { return a.name.localizedStandardCompare(b.name) == .orderedAscending }
            return a.id < b.id
        }
        return Array(sorted.prefix(DeviceHomeLimits.maxItems))
    }

    /// 特性の並びを `DeviceHomeTrait.allCases` に揃え、同じ特性が 2 つ来たら先に読めた方を使う。
    static func readings(_ all: [DeviceHomeReading]) -> [DeviceHomeReading] {
        var byTrait: [DeviceHomeTrait: Double] = [:]
        for r in all where byTrait[r.trait] == nil { byTrait[r.trait] = r.value }
        return DeviceHomeTrait.allCases.compactMap { t in byTrait[t].map { DeviceHomeReading(trait: t, value: $0) } }
    }
}

// MARK: - 偽物

/// `METAMON_MOCK=1`・テスト・プレビュー用。中身と許可の状態を外から決められる。
public struct FakeDeviceHome: DeviceHomeSource {
    public var access: DeviceHomeAccess
    /// `requesting: true` で未決定の時に、ダイアログで選ばれたことにする状態
    public var grantOnRequest: DeviceHomeAccess
    public var allHomes: [DeviceHomeInfo]
    public var allRooms: [DeviceRoom]
    public var allAccessories: [DeviceAccessory]

    public init(access: DeviceHomeAccess = .authorized,
                grantOnRequest: DeviceHomeAccess = .authorized,
                homes: [DeviceHomeInfo] = [], rooms: [DeviceRoom] = [], accessories: [DeviceAccessory] = []) {
        self.access = access
        self.grantOnRequest = grantOnRequest
        self.allHomes = homes
        self.allRooms = rooms
        self.allAccessories = accessories
    }

    public func authorization(requesting: Bool) async -> DeviceHomeAccess {
        requesting && access == .notDetermined ? grantOnRequest : access
    }

    public func homes() async throws -> [DeviceHomeInfo] {
        try check()
        return DeviceHomeQuery.homes(allHomes)
    }

    public func rooms(homeId: String?) async throws -> [DeviceRoom] {
        try check()
        guard let id = DeviceHomeQuery.resolvedHomeId(homeId, in: allHomes) else { throw DeviceHomeError.noHome }
        return DeviceHomeQuery.rooms(allRooms, homeId: id)
    }

    public func accessories(homeId: String?, roomId: String?) async throws -> [DeviceAccessory] {
        try check()
        guard let id = DeviceHomeQuery.resolvedHomeId(homeId, in: allHomes) else { throw DeviceHomeError.noHome }
        return DeviceHomeQuery.accessories(allAccessories, homeId: id, roomId: roomId)
    }

    public func changes() -> AsyncStream<Void> { AsyncStream { $0.finish() } }

    private func check() throws {
        guard access.canRead else { throw DeviceHomeError.notAuthorized(access) }
    }

    /// `METAMON_MOCK=1` の固定の中身。
    public static var sample: FakeDeviceHome {
        let home = DeviceHomeInfo(id: "fake-home", name: "自宅", isPrimary: true)
        let living = DeviceRoom(id: "fake-living", name: "リビング", homeId: home.id)
        let bedroom = DeviceRoom(id: "fake-bedroom", name: "寝室", homeId: home.id)
        return FakeDeviceHome(
            homes: [home],
            rooms: [living, bedroom],
            accessories: [
                DeviceAccessory(id: "a1", name: "リビングのライト", kind: .light, homeId: home.id,
                                roomId: living.id, roomName: living.name, isReachable: true,
                                readings: [DeviceHomeReading(trait: .power, value: 1), DeviceHomeReading(trait: .brightness, value: 70)]),
                DeviceAccessory(id: "a2", name: "エアコン", kind: .airConditioner, homeId: home.id,
                                roomId: living.id, roomName: living.name, isReachable: true,
                                readings: [DeviceHomeReading(trait: .power, value: 1), DeviceHomeReading(trait: .temperature, value: 24.5)]),
                DeviceAccessory(id: "a3", name: "寝室の温湿度計", kind: .sensor, homeId: home.id,
                                roomId: bedroom.id, roomName: bedroom.name, isReachable: true,
                                readings: [DeviceHomeReading(trait: .temperature, value: 21.3), DeviceHomeReading(trait: .humidity, value: 48)]),
            ]
        )
    }
}

// MARK: - 実体

#if os(iOS)
/// HomeKit から読む。`HMHomeManager` は 1 つだけ持つ(作ると許可を求める・変更通知もこの manager から来る)。
/// Info.plist に説明文が無いビルド(審査用の版など)では manager を作らず `.restricted` を返す。
public final class LiveDeviceHome: DeviceHomeSource, @unchecked Sendable {
    static let usageKey = "NSHomeKitUsageDescription"

    private let store = HomeStore()

    public init() {}

    /// 説明文の有無は生の Info.plist で見る(`LiveDeviceCalendar.hasUsageDescription` と同じ理由)。
    static func hasUsageDescription(in info: [String: Any]?) -> Bool {
        guard let text = info?[usageKey] as? String else { return false }
        return !text.isEmpty
    }

    public func authorization(requesting: Bool) async -> DeviceHomeAccess {
        guard Self.hasUsageDescription(in: Bundle.main.infoDictionary) else { return .restricted }
        let current = await store.currentAccess()
        guard requesting || current != .notDetermined else { return current }
        return await store.loadedAccess()
    }

    public func homes() async throws -> [DeviceHomeInfo] {
        try await ready()
        return DeviceHomeQuery.homes(await store.homeInfos())
    }

    public func rooms(homeId: String?) async throws -> [DeviceRoom] {
        try await ready()
        guard let id = DeviceHomeQuery.resolvedHomeId(homeId, in: await store.homeInfos()) else { throw DeviceHomeError.noHome }
        return DeviceHomeQuery.rooms(await store.roomList(homeId: id), homeId: id)
    }

    public func accessories(homeId: String?, roomId: String?) async throws -> [DeviceAccessory] {
        try await ready()
        guard let id = DeviceHomeQuery.resolvedHomeId(homeId, in: await store.homeInfos()) else { throw DeviceHomeError.noHome }
        let found = await store.accessoryList(homeId: id, roomId: roomId)
        return DeviceHomeQuery.accessories(found, homeId: id, roomId: roomId)
    }

    public func changes() -> AsyncStream<Void> {
        AsyncStream { continuation in
            Task { await store.addObserver(continuation) }
        }
    }

    private func ready() async throws {
        guard Self.hasUsageDescription(in: Bundle.main.infoDictionary) else { throw DeviceHomeError.notAuthorized(.restricted) }
        let access = await store.loadedAccess()
        guard access.canRead else { throw DeviceHomeError.notAuthorized(access) }
    }
}

/// `HMHomeManager` を抱えて、家が読み込まれるまで待つ。HomeKit は主スレッドで触る。
@MainActor
private final class HomeStore: NSObject, HMHomeManagerDelegate {
    private var manager: HMHomeManager?
    private var loaded = false
    private var waiting: [CheckedContinuation<Void, Never>] = []
    private var observers: [UUID: AsyncStream<Void>.Continuation] = [:]

    nonisolated override init() { super.init() }

    /// 読み込みを待てる時間。これを過ぎたら今ある中身で答える(家が無い端末は通知が来ないことがある)。
    private static let loadTimeout: Duration = .seconds(3)

    /// `HMHomeManager` を作らずに分かる状態。未決定なら `.notDetermined`。
    func currentAccess() -> DeviceHomeAccess {
        manager.map { Self.access($0.authorizationStatus) } ?? .notDetermined
    }

    /// manager を作り(初回は許可ダイアログ)、家が読み込まれるまで待ってから状態を返す。
    func loadedAccess() async -> DeviceHomeAccess {
        await load()
        return manager.map { Self.access($0.authorizationStatus) } ?? .restricted
    }

    func homeInfos() -> [DeviceHomeInfo] {
        guard let manager else { return [] }
        let primaryId = manager.primaryHome?.uniqueIdentifier
        return manager.homes.map {
            DeviceHomeInfo(id: $0.uniqueIdentifier.uuidString, name: $0.name, isPrimary: $0.uniqueIdentifier == primaryId)
        }
    }

    func roomList(homeId: String) -> [DeviceRoom] {
        guard let home = home(homeId) else { return [] }
        return home.rooms.map { DeviceRoom(id: $0.uniqueIdentifier.uuidString, name: $0.name, homeId: homeId) }
    }

    func accessoryList(homeId: String, roomId: String?) async -> [DeviceAccessory] {
        guard let home = home(homeId) else { return [] }
        let wanted = home.accessories.filter { a in
            roomId.map { $0 == a.room?.uniqueIdentifier.uuidString } ?? true
        }
        var out: [DeviceAccessory] = []
        for a in wanted.prefix(DeviceHomeLimits.maxItems) {
            out.append(DeviceAccessory(
                id: a.uniqueIdentifier.uuidString,
                name: a.name,
                kind: Self.kind(a.category.categoryType),
                homeId: homeId,
                roomId: a.room?.uniqueIdentifier.uuidString,
                roomName: a.room?.name,
                isReachable: a.isReachable,
                readings: DeviceHomeQuery.readings(await Self.readings(a))
            ))
        }
        return out
    }

    func addObserver(_ continuation: AsyncStream<Void>.Continuation) {
        let key = UUID()
        observers[key] = continuation
        continuation.onTermination = { [weak self] _ in
            Task { @MainActor in self?.observers[key] = nil }
        }
    }

    // MARK: HMHomeManagerDelegate

    nonisolated func homeManagerDidUpdateHomes(_ manager: HMHomeManager) {
        Task { @MainActor in self.finishLoading() }
    }

    nonisolated func homeManager(_ manager: HMHomeManager, didAdd home: HMHome) {
        Task { @MainActor in self.notifyObservers() }
    }

    nonisolated func homeManager(_ manager: HMHomeManager, didRemove home: HMHome) {
        Task { @MainActor in self.notifyObservers() }
    }

    // MARK: 中身

    private func home(_ id: String) -> HMHome? {
        manager?.homes.first { $0.uniqueIdentifier.uuidString == id }
    }

    private func load() async {
        if manager == nil {
            let m = HMHomeManager()
            m.delegate = self
            manager = m
        }
        guard !loaded else { return }
        await withTaskGroup(of: Void.self) { group in
            group.addTask { @MainActor in
                await withCheckedContinuation { self.waiting.append($0) }
            }
            group.addTask {
                try? await Task.sleep(for: Self.loadTimeout)
                await MainActor.run { self.finishLoading() }
            }
            await group.next()
            group.cancelAll()
        }
    }

    private func finishLoading() {
        loaded = true
        let pending = waiting
        waiting = []
        for c in pending { c.resume() }
        notifyObservers()
    }

    private func notifyObservers() {
        for c in observers.values { c.yield() }
    }

    private static func access(_ status: HMHomeManagerAuthorizationStatus) -> DeviceHomeAccess {
        if status.contains(.authorized) { return .authorized }
        if status.contains(.restricted) { return .restricted }
        return .notDetermined
    }

    private static func kind(_ category: String) -> DeviceAccessoryKind {
        if #available(iOS 18.0, *) {
            switch category {
            case HMAccessoryCategoryTypeSpeaker, HMAccessoryCategoryTypeAudioReceiver: return .speaker
            case HMAccessoryCategoryTypeWiFiRouter: return .bridge
            default: break
            }
        }
        return switch category {
        case HMAccessoryCategoryTypeLightbulb: .light
        case HMAccessoryCategoryTypeOutlet: .outlet
        case HMAccessoryCategoryTypeSwitch, HMAccessoryCategoryTypeProgrammableSwitch: .powerSwitch
        case HMAccessoryCategoryTypeThermostat: .thermostat
        case HMAccessoryCategoryTypeAirConditioner, HMAccessoryCategoryTypeAirHeater: .airConditioner
        case HMAccessoryCategoryTypeFan: .fan
        case HMAccessoryCategoryTypeAirPurifier: .airPurifier
        case HMAccessoryCategoryTypeAirHumidifier, HMAccessoryCategoryTypeAirDehumidifier: .humidifier
        case HMAccessoryCategoryTypeSensor: .sensor
        case HMAccessoryCategoryTypeDoorLock: .lock
        case HMAccessoryCategoryTypeWindowCovering, HMAccessoryCategoryTypeWindow, HMAccessoryCategoryTypeDoor: .windowCovering
        case HMAccessoryCategoryTypeIPCamera, HMAccessoryCategoryTypeVideoDoorbell: .camera
        case HMAccessoryCategoryTypeBridge, HMAccessoryCategoryTypeRangeExtender: .bridge
        default: .other
        }
    }

    private static func trait(_ characteristic: String) -> DeviceHomeTrait? {
        switch characteristic {
        case HMCharacteristicTypePowerState: .power
        case HMCharacteristicTypeBrightness: .brightness
        case HMCharacteristicTypeCurrentTemperature: .temperature
        case HMCharacteristicTypeCurrentRelativeHumidity: .humidity
        default: nil
        }
    }

    /// よく使う特性の今の値を読む。読めなかったものは落とす(機器が切れている時など)。
    private static func readings(_ accessory: HMAccessory) async -> [DeviceHomeReading] {
        var out: [DeviceHomeReading] = []
        for service in accessory.services {
            for c in service.characteristics {
                guard let t = trait(c.characteristicType) else { continue }
                try? await c.readValue()
                guard let value = numeric(c.value) else { continue }
                out.append(DeviceHomeReading(trait: t, value: value))
            }
        }
        return out
    }

    private static func numeric(_ value: Any?) -> Double? {
        switch value {
        case let b as Bool: b ? 1 : 0
        case let n as NSNumber: n.doubleValue
        default: nil
        }
    }
}
#else
/// iOS 以外(mac アプリ・テスト)。HomeKit が無いので常に「使えません」。
public final class LiveDeviceHome: DeviceHomeSource {
    public init() {}

    public func authorization(requesting: Bool) async -> DeviceHomeAccess { .restricted }
    public func homes() async throws -> [DeviceHomeInfo] { throw DeviceHomeError.unavailable }
    public func rooms(homeId: String?) async throws -> [DeviceRoom] { throw DeviceHomeError.unavailable }
    public func accessories(homeId: String?, roomId: String?) async throws -> [DeviceAccessory] { throw DeviceHomeError.unavailable }
    public func changes() -> AsyncStream<Void> { AsyncStream { $0.finish() } }
}
#endif
#endif
