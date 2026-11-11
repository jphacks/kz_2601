import SwiftUI
import AppKit
import Observation
import UserNotifications
import MetamonDSL
import MetamonRenderer
import MetamonServices

/// Mac のローカル通知(#965)。予約・くり返し・上限・識別子は iOS と同じ `UserNotificationScheduler`
/// (`NotificationPlanner`)を使い、Mac の通知センターに出す。iOS の `AppRuntime+Notifications` と同じ流れ:
/// - DSL の `notifications`: 開いた時・記録が変わった時・Mac が前面に戻った時に予約し直す
/// - 条件の通知(`when`): 記録が変わったら評価し、偽 → 真で 1 回出す
/// - ボタンの `scheduleNotification` / `cancelNotification`
/// アラーム(止めるまで鳴る・AlarmKit)は Mac に無いので、音つきの普通の通知で届ける(iOS 26.1 未満と同じ)。
/// 場所の通知(`near`)は Mac では見張らない(持ち歩かないため)。使おうとしたら一言で知らせる。
@MainActor
@Observable
final class MacRuntimeNotifications {
    /// 全アプリで 1 つ(予約の登録簿と直列の列は scheduler の中で共有)。alarms を渡さない = 普通の通知
    static let scheduler: any NotificationScheduler = UserNotificationScheduler()

    /// 通知を使うアプリで、Mac の通知が許可されていない(上にバーを出す)
    private(set) var denied = false
    /// バーを閉じた(このアプリを開いている間は出さない)
    var barDismissed = false
    /// このアプリの次の通知(近い順・#1325)。予約し直すたびに `NotificationSyncResult.upcoming` で置き換える
    private(set) var upcoming: [UpcomingNotification] = []
    /// ツールバーの「次の通知」のポップオーバー(「アプリ」メニューからも開く)
    var showsUpcoming = false

    @ObservationIgnored private weak var runtime: MacAppRuntime?
    @ObservationIgnored private var syncTask: Task<Void, Never>?
    @ObservationIgnored private var conditionTask: Task<Void, Never>?
    @ObservationIgnored private var activeObserver: NSObjectProtocol?
    @ObservationIgnored private var toldPlacesUnavailable = false

    private var scheduler: any NotificationScheduler { Self.scheduler }

    /// 消したアプリ(#1324)。開いていたランタイムの遅れた予約し直しで、消した予約を戻さない(appId は使い回さない)
    private static var removedAppIds: Set<String> = []

    /// アプリを消した時: そのアプリの通知とタイマーの予約を消す(iOS の削除と同じ `AppNotificationCleanup`)。
    static func removeApp(_ appId: String) async {
        removedAppIds.insert(appId)
        await AppNotificationCleanup.removeApp(appId, notifications: scheduler)
    }

    /// 見ているランタイム。消したアプリなら nil(何も予約しない)
    private var liveRuntime: MacAppRuntime? {
        guard let runtime, !Self.removedAppIds.contains(runtime.appId) else { return nil }
        return runtime
    }

    /// ランタイムを作った時に 1 回。
    func start(_ runtime: MacAppRuntime) {
        self.runtime = runtime
        observeRecords()
        Task { await syncNow() }
        if specs.contains(where: \.isConditional) { scheduleConditionCheck() }
        activeObserver = NotificationCenter.default.addObserver(
            forName: NSApplication.didBecomeActiveNotification, object: nil, queue: .main
        ) { [weak self] _ in
            Task { @MainActor in await self?.refresh() } // 届いた 1 回きりの空き枠を埋める
        }
    }

    deinit {
        if let activeObserver { NotificationCenter.default.removeObserver(activeObserver) }
    }

    private var specs: [NotificationSpec] { runtime?.dsl.app.notifications ?? [] }

    // MARK: - ボタン

    func perform(_ action: Action) async {
        guard let runtime = liveRuntime else { return }
        switch action.type {
        case .scheduleNotification:
            guard let spec = action.notification else { return }
            guard !spec.isNearBased else {
                runtime.toast = Self.placesUnavailable
                return
            }
            do {
                let next = try await scheduler.schedule(spec, appId: runtime.appId)
                await refresh()
                runtime.toast = next.map { L10n.tr("通知を予約しました(\(NotificationCopy.when($0)))") }
                    ?? L10n.tr("通知を予約できませんでした")
            } catch {
                runtime.toast = L10n.tr("通知を予約できませんでした")
            }
        case .cancelNotification:
            guard let id = action.notificationId else { return }
            await scheduler.cancel(id: id, appId: runtime.appId)
            await refresh()
        default:
            break
        }
    }

    static var placesUnavailable: String { L10n.tr("場所の通知は Mac では使えません") }

    // MARK: - DSL の通知

    /// 記録の変化を見張る(続けて変わっても 1 回にまとめる)。
    private func observeRecords() {
        guard let runtime else { return }
        withObservationTracking {
            _ = runtime.records
        } onChange: { [weak self] in
            Task { @MainActor in
                guard let self else { return }
                if self.specs.contains(where: { $0.recordEntity != nil }) { self.scheduleSync() }
                if self.specs.contains(where: \.isConditional) { self.scheduleConditionCheck() }
                self.observeRecords()
            }
        }
    }

    private func scheduleSync() {
        syncTask?.cancel()
        syncTask = Task { [weak self] in
            try? await Task.sleep(for: .milliseconds(400))
            guard let self, !Task.isCancelled else { return }
            await self.syncNow()
        }
    }

    func syncNow() async {
        guard let runtime = liveRuntime else { return }
        let specs = self.specs
        let recs = await records(for: Set(specs.compactMap(\.recordEntity)), runtime)
        guard liveRuntime != nil else { return } // 読んでいる間に消された: 消した予約を戻さない
        let result = await scheduler.sync(appId: runtime.appId, specs: specs, records: recs)
        handle(result, hasSpecs: !specs.isEmpty)
    }

    func refresh() async {
        guard let runtime = liveRuntime else { return }
        let result = await scheduler.refresh(appId: runtime.appId)
        handle(result, hasSpecs: !specs.isEmpty)
    }

    private func handle(_ result: NotificationSyncResult, hasSpecs: Bool) {
        denied = result.denied && hasSpecs
        upcoming = result.upcoming
        // 場所の通知が保存されているのに Mac では届かない: 1 回だけ知らせる
        if result.locationOff, hasSpecs, !toldPlacesUnavailable {
            toldPlacesUnavailable = true
            runtime?.toast = Self.placesUnavailable
        }
    }

    // MARK: - 条件の通知

    private func scheduleConditionCheck() {
        conditionTask?.cancel()
        conditionTask = Task { [weak self] in
            try? await Task.sleep(for: .milliseconds(300))
            guard let self, !Task.isCancelled else { return }
            await self.checkConditionsNow()
        }
    }

    /// iOS と同じ: 未表示の entity は端末内 DB から読む(読み込み前の空で評価して誤って鳴らさない)。
    func checkConditionsNow() async {
        guard let runtime = liveRuntime else { return }
        let dsl = runtime.dsl
        let specs = dsl.app.notifications.filter(\.isConditional)
        guard !specs.isEmpty else { return }
        let recs = await records(for: Set(dsl.entities.map(\.id)), runtime)
        guard liveRuntime != nil else { return } // 読んでいる間に消された: 消したアプリの通知を出さない
        let current = ConditionNotifications.evaluate(specs, dsl: dsl, records: recs)
        let fire = await ConditionNotificationStore.shared.update(appId: runtime.appId, current: current)
        for spec in specs where fire.contains(spec.id) {
            await scheduler.post(spec, appId: runtime.appId)
        }
    }

    private func records(for entities: Set<String>, _ runtime: MacAppRuntime) async -> [String: [Record]] {
        var recs: [String: [Record]] = [:]
        for entity in entities {
            if let loaded = runtime.records[entity] {
                recs[entity] = loaded
            } else {
                recs[entity] = await runtime.recordSync.cached(appId: runtime.appId, entity: entity)
            }
        }
        return recs
    }
}

// MARK: - 通知センターの代理

/// 前面でも通知を出し、押されたらそのアプリを開く(#965)。起動の最初に `install()`(押されて起動した時も受ける)。
final class MacNotificationDelegate: NSObject, UNUserNotificationCenterDelegate {
    static let shared = MacNotificationDelegate()

    static func install() {
        UNUserNotificationCenter.current().delegate = shared
    }

    func userNotificationCenter(
        _ center: UNUserNotificationCenter, willPresent notification: UNNotification
    ) async -> UNNotificationPresentationOptions {
        // 終わったタイマーが画面に見えている時は出さない(iOS と同じ・#38)
        let id = notification.request.identifier
        if await MainActor.run(body: { TimerVisibility.isShowing(notificationId: id) }) { return [] }
        return [.banner, .list, .sound]
    }

    func userNotificationCenter(
        _ center: UNUserNotificationCenter, didReceive response: UNNotificationResponse
    ) async {
        let id = response.notification.request.identifier
        #if !APP_STORE_REVIEW
        // プッシュ(#982): 本文の appId のアプリを開く。ローカル通知には appId が無いので下の識別子で開く
        if response.actionIdentifier == UNNotificationDefaultActionIdentifier,
           let appId = PushPayload.appId(from: response.notification.request.content.userInfo) {
            await MainActor.run { NSApp.activate() }
            await MacPushHost.shared.open(appId: appId)
            return
        }
        #endif
        await MainActor.run {
            NSApp.activate()
            MacNotificationRouter.shared.open(notificationId: id)
        }
    }
}

/// 押された通知を一覧の選択に渡す(ウインドウがまだ無い時は出来てから受け取る)。
@MainActor
@Observable
final class MacNotificationRouter {
    static let shared = MacNotificationRouter()
    private(set) var pending: Route?

    /// 押された通知の行き先
    enum Route: Equatable {
        /// ローカル通知の識別子(`dsl.` / `when.` / `timer.` + appId・#965)
        case notification(String)
        /// プッシュの本文の appId(#982)
        case app(String)

        /// 一覧で選ぶアプリ。一覧に無ければ nil(前に出すだけ)
        func appId(among ids: [String]) -> String? {
            switch self {
            case .notification(let id): NotificationAppRoute.appId(for: id, among: ids)
            case .app(let id): ids.contains(id) ? id : nil
            }
        }
    }

    func open(notificationId: String) { pending = .notification(notificationId) }

    func open(appId: String) { pending = .app(appId) }

    func take() -> Route? {
        defer { pending = nil }
        return pending
    }
}

extension View {
    /// 押された通知のアプリを一覧で選ぶ(一覧に無ければ前に出すだけ)。
    func macNotificationRouting(selection: Binding<String?>, appIds: [String]) -> some View {
        modifier(MacNotificationRouting(selection: selection, appIds: appIds))
    }

    /// 通知を使うアプリで Mac の通知が許可されていない時、上に「通知がオフ」のバーを出す。
    func macNotificationsOffBar(_ notifications: MacRuntimeNotifications) -> some View {
        modifier(MacNotificationsOffBar(notifications: notifications))
    }
}

private struct MacNotificationRouting: ViewModifier {
    @Binding var selection: String?
    let appIds: [String]
    private var router: MacNotificationRouter { .shared }

    func body(content: Content) -> some View {
        content
            .onAppear(perform: route)
            .onChange(of: router.pending) { route() }
    }

    private func route() {
        guard let route = router.take() else { return }
        if let appId = route.appId(among: appIds) { selection = appId }
    }
}

private struct MacNotificationsOffBar: ViewModifier {
    @Bindable var notifications: MacRuntimeNotifications

    private var visible: Bool { notifications.denied && !notifications.barDismissed }

    func body(content: Content) -> some View {
        content
            .safeAreaInset(edge: .top, spacing: 0) {
                if visible {
                    bar
                        .macTopTransition()
                        .onAppear { MacVoiceOver.announce(Self.message) }
                }
            }
            .macAnimation(.easeInOut(duration: 0.25), value: visible)
    }

    private static var message: String { L10n.tr("通知がオフのため、このアプリの通知は届きません") }

    private var bar: some View {
        HStack(spacing: 10) {
            Image(systemName: "bell.slash")
                .foregroundStyle(Theme.Colors.warning)
                .accessibilityHidden(true)
            Text(Self.message)
                .font(.callout)
                .frame(maxWidth: .infinity, alignment: .leading)
            Button("設定を開く", action: openSettings)
                .accessibilityIdentifier("notificationsOff.openSettings")
            Button { notifications.barDismissed = true } label: { Image(systemName: "xmark") }
                .buttonStyle(.plain)
                .macSecondaryText() // コントラストを上げる時は本文の色(#1206)
                .help("閉じる")
                .accessibilityLabel("閉じる")
                .accessibilityIdentifier("notificationsOff.dismiss")
        }
        .padding(.horizontal, 14).padding(.vertical, 8)
        .macBarSurface() // 透明度を下げる・コントラストを上げる(#1206)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("notificationsOff")
    }

    /// システム設定の「通知」のこのアプリの欄
    private func openSettings() {
        let bundleId = Bundle.main.bundleIdentifier ?? ""
        if let url = URL(string: "x-apple.systempreferences:com.apple.Notifications-Settings.extension?id=\(bundleId)") {
            NSWorkspace.shared.open(url)
        }
    }
}
