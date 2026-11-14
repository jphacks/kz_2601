import Foundation
import MetamonRenderer
import MetamonServices

/// タイマー部品(#38・#423)の端末内の状態。キーは appId で始まる(同じ名前のアプリが 2 つあっても混ざらない)。
extension AppRuntime {
    var appId: String { document.appId }

    /// 古いキー(アプリ名で始まる)からの 1 回きりの移行。起動時に部品が描かれる前に呼ぶ。
    /// 持ち主はアプリ一覧のキャッシュと開いているアプリから決め、決められない分は捨てる。
    func migrateTimerKeysIfNeeded(defaults: UserDefaults = .standard, apps cached: [AppSummary] = AppListCache.shared.load()) {
        let current = TimerKeyMigration.App(id: document.appId, name: dsl.app.name)
        let result = TimerKeyMigration.migrateStore(defaults, apps: cached.map { .init(id: $0.id, name: $0.name) }, current: current)
        guard !result.moved.isEmpty || !result.dropped.isEmpty else { return }
        print("[Metamon] timer keys migrated: moved=\(result.moved.count) dropped=\(result.dropped.count)")
        Task { await TimerKeyMigration.migrateNotifications(result) }
        // 古いキーのロック画面の表示は消す(部品を開くと新しいキーで出し直す)
        LiveActivities.enqueue { await LiveActivities.endLegacyTimers() }
    }
}
