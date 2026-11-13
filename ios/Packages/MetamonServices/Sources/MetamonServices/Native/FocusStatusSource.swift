import Foundation
#if canImport(Intents)
import Intents
#endif
#if os(iOS)
import UIKit
#elseif os(macOS)
import AppKit
#endif

// 集中モード中か(#1990・親 #1799)。式・スキーマ・AI の決まりにはまだ出さない(#1775 の決定待ち)。
// 許可の画面を出すきっかけも決定の後。取れるのは「集中中か」だけ(モードの名前は取れない)。値はサーバーに送らない。

/// 集中モードの状態を読む許可。
public enum FocusAuthorization: String, Equatable, Sendable, CaseIterable {
    case notDetermined
    case denied
    /// 保護者の制限・審査用の版など。entitlement(Communication Notifications)が無いビルドもこれになる想定(未確認)
    case restricted
    case authorized
}

/// 集中モードの状態。
public struct FocusStatus: Equatable, Sendable {
    public var authorization: FocusAuthorization
    /// 集中中か。許可が無い時・OS が分からない時は nil
    public var isFocused: Bool?

    public init(authorization: FocusAuthorization, isFocused: Bool?) {
        self.authorization = authorization
        self.isFocused = authorization == .authorized ? isFocused : nil
    }

    /// 許可が無い時は OS の値を読まない(読んでも nil にする)
    public static func read(authorization: FocusAuthorization, isFocused: () -> Bool?) -> FocusStatus {
        FocusStatus(authorization: authorization, isFocused: authorization == .authorized ? isFocused() : nil)
    }

    public static let restricted = FocusStatus(authorization: .restricted, isFocused: nil)
}

#if canImport(Intents)
extension FocusAuthorization {
    init(_ status: INFocusStatusAuthorizationStatus) {
        switch status {
        case .notDetermined: self = .notDetermined
        case .denied: self = .denied
        case .restricted: self = .restricted
        case .authorized: self = .authorized
        @unknown default: self = .restricted
        }
    }
}
#endif

/// 監視を止めるための札。`cancel()` を呼ぶか手放すと止まる。
@MainActor
public final class FocusStatusObservation {
    private var onCancel: (@MainActor () -> Void)?

    init(onCancel: @escaping @MainActor () -> Void) { self.onCancel = onCancel }

    public func cancel() {
        let f = onCancel
        onCancel = nil
        f?()
    }

    deinit {
        if let f = onCancel { Task { @MainActor in f() } }
    }
}

/// 集中モード中かを読む口。
@MainActor
public protocol FocusStatusSource: AnyObject {
    /// 今の値(許可を求めない)
    var current: FocusStatus { get }
    /// 許可の状態。`requesting` が true で未決定なら OS の許可の画面を出して結果を返す
    func authorization(requesting: Bool) async -> FocusAuthorization
    /// 変わるたびに呼ぶ(登録した時に今の値を 1 回呼ぶ)。札を cancel すると止まる。
    func observe(_ onChange: @escaping @MainActor (FocusStatus) -> Void) -> FocusStatusObservation
}

public extension FocusStatusSource {
    /// 集中中か。許可が無ければ nil
    var isFocused: Bool? { current.isFocused }

    /// `observe` の AsyncStream 版。ストリームを捨てると監視も止まる。
    func updates() -> AsyncStream<FocusStatus> {
        AsyncStream { continuation in
            let observation = observe { continuation.yield($0) }
            continuation.onTermination = { _ in
                Task { @MainActor in observation.cancel() }
            }
        }
    }
}

// MARK: - 実体

/// OS から読む部分(テストで差し替える)。
@MainActor
struct FocusStatusReadings {
    var authorization: () -> FocusAuthorization
    var requestAuthorization: (@escaping @Sendable (FocusAuthorization) -> Void) -> Void
    var isFocused: () -> Bool?
    /// 読み直すきっかけの通知(アプリが前に来た時)。OS は集中モードの変化を知らせないので、前に来るたびに読む
    var notificationNames: [Notification.Name]
}

/// `INFocusStatusCenter.default` を読む実体。アプリが前に来た時に読み直し、変わっていれば知らせる。
@MainActor
public final class LiveFocusStatusSource: FocusStatusSource {
    private let readings: FocusStatusReadings
    private let center: NotificationCenter
    private var subscribers: [UUID: @MainActor (FocusStatus) -> Void] = [:]
    private var tokens: [NSObjectProtocol] = []
    private var last: FocusStatus?

    public convenience init() {
        self.init(readings: .system, center: .default)
    }

    init(readings: FocusStatusReadings, center: NotificationCenter) {
        self.readings = readings
        self.center = center
    }

    public var current: FocusStatus {
        .read(authorization: readings.authorization(), isFocused: readings.isFocused)
    }

    public func authorization(requesting: Bool) async -> FocusAuthorization {
        let now = readings.authorization()
        guard requesting, now == .notDetermined else { return now }
        let request = readings.requestAuthorization
        let result = await withCheckedContinuation { (continuation: CheckedContinuation<FocusAuthorization, Never>) in
            request { continuation.resume(returning: $0) }
        }
        refresh()
        return result
    }

    public func observe(_ onChange: @escaping @MainActor (FocusStatus) -> Void) -> FocusStatusObservation {
        let id = UUID()
        if subscribers.isEmpty {
            start()
        } else {
            // 前から見ている側にも、新しい値を先に知らせる(last だけ進めて取り残さない)
            refresh()
        }
        subscribers[id] = onChange
        let state = current
        last = state
        onChange(state)
        return FocusStatusObservation { self.remove(id) }
    }

    /// 監視中か(テスト用)
    var isObserving: Bool { !tokens.isEmpty }

    private func start() {
        tokens = readings.notificationNames.map { name in
            center.addObserver(forName: name, object: nil, queue: nil) { [weak self] _ in
                if Thread.isMainThread {
                    MainActor.assumeIsolated { self?.refresh() }
                } else {
                    Task { @MainActor in self?.refresh() }
                }
            }
        }
    }

    private func remove(_ id: UUID) {
        guard subscribers.removeValue(forKey: id) != nil, subscribers.isEmpty else { return }
        tokens.forEach(center.removeObserver)
        tokens = []
        last = nil
    }

    func refresh() {
        guard !subscribers.isEmpty else { return }
        let state = current
        guard state != last else { return }
        last = state
        for f in subscribers.values { f(state) }
    }
}

extension FocusStatusReadings {
    static var system: FocusStatusReadings {
        #if os(iOS) || os(macOS)
        // Mac(#2328)も同じ INFocusStatusCenter(macOS 12 以降)。読み直すのはアプリが前に来た時
        FocusStatusReadings(
            authorization: { FocusAuthorization(INFocusStatusCenter.default.authorizationStatus) },
            requestAuthorization: { done in
                // entitlement が無いビルドでは restricted が返る想定(未確認・実機で確かめる)
                INFocusStatusCenter.default.requestAuthorization { done(FocusAuthorization($0)) }
            },
            isFocused: { INFocusStatusCenter.default.focusStatus.isFocused },
            notificationNames: [didBecomeActive]
        )
        #else
        // それ以外の OS は対象外。常に restricted / nil
        FocusStatusReadings(
            authorization: { .restricted },
            requestAuthorization: { $0(.restricted) },
            isFocused: { nil },
            notificationNames: []
        )
        #endif
    }

    /// アプリが前に来た時の通知
    static var didBecomeActive: Notification.Name {
        #if os(iOS)
        UIApplication.didBecomeActiveNotification
        #elseif os(macOS)
        NSApplication.didBecomeActiveNotification
        #else
        Notification.Name("FocusStatusReadings.didBecomeActive")
        #endif
    }
}

// MARK: - 偽物

/// テスト・`METAMON_MOCK=1`・DEBUG で使う偽物。`authorizationStatus` / `focused` を書き換えると監視している側に届く。
@MainActor
public final class FakeFocusStatusSource: FocusStatusSource {
    public var authorizationStatus: FocusAuthorization { didSet { notify() } }
    /// OS が返す生の値(許可が無ければ外には nil で見える)
    public var focused: Bool? { didSet { notify() } }
    /// 未決定の時に許可を求めたらこれになる
    public var grantOnRequest: FocusAuthorization
    /// 許可を求めた回数(テスト用)
    public private(set) var requestCount = 0
    private var subscribers: [UUID: @MainActor (FocusStatus) -> Void] = [:]
    private var last: FocusStatus

    public init(authorization: FocusAuthorization = .notDetermined, focused: Bool? = false,
                grantOnRequest: FocusAuthorization = .authorized) {
        self.authorizationStatus = authorization
        self.focused = focused
        self.grantOnRequest = grantOnRequest
        self.last = FocusStatus(authorization: authorization, isFocused: focused)
    }

    /// 環境変数で初期値を決める: `METAMON_FOCUS_AUTH`(notDetermined/denied/restricted/authorized・無ければ authorized)・`METAMON_FOCUSED=1`
    public convenience init(environment: [String: String]) {
        let auth = environment["METAMON_FOCUS_AUTH"].flatMap(FocusAuthorization.init(rawValue:)) ?? .authorized
        self.init(authorization: auth, focused: environment["METAMON_FOCUSED"] == "1")
    }

    public var current: FocusStatus { FocusStatus(authorization: authorizationStatus, isFocused: focused) }

    public func authorization(requesting: Bool) async -> FocusAuthorization {
        if requesting, authorizationStatus == .notDetermined {
            requestCount += 1
            authorizationStatus = grantOnRequest
        }
        return authorizationStatus
    }

    /// 見ている数(テスト用)
    public var observerCount: Int { subscribers.count }

    public func observe(_ onChange: @escaping @MainActor (FocusStatus) -> Void) -> FocusStatusObservation {
        let id = UUID()
        subscribers[id] = onChange
        onChange(current)
        return FocusStatusObservation { self.subscribers[id] = nil }
    }

    private func notify() {
        let state = current
        guard state != last else { return }
        last = state
        subscribers.values.forEach { $0(state) }
    }
}

// MARK: - 選ぶ

public enum FocusStatusSources {
    /// 審査用の版(`reviewBuild`。アプリは `APP_STORE_REVIEW` を渡す)は常に restricted の偽物で、OS の許可を求めない
    /// (説明文 NSFocusStatusUsageDescription を strip-review-plist.sh が消すため)。
    /// それ以外は `METAMON_MOCK=1`、または DEBUG で `METAMON_FAKE_FOCUS=1` なら偽物、それ以外は OS の実体。
    @MainActor
    public static func make(reviewBuild: Bool,
                            environment: [String: String] = ProcessInfo.processInfo.environment) -> FocusStatusSource {
        if reviewBuild {
            return FakeFocusStatusSource(authorization: .restricted, focused: nil, grantOnRequest: .restricted)
        }
        var fake = environment["METAMON_MOCK"] == "1"
        #if DEBUG
        fake = fake || environment["METAMON_FAKE_FOCUS"] == "1"
        #endif
        return fake ? FakeFocusStatusSource(environment: environment) : LiveFocusStatusSource()
    }
}
