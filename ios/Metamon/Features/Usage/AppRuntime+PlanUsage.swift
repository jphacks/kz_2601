import Foundation
import MetamonServices

/// 使っている量(#317・#630)。設定の「使っている量」と、上限に近い時の 1 行のお知らせが読む。
/// 量はサーバー(`GET /v1/me/usage`・`/v1/apps/{appId}/usage`)から 5 分だけキャッシュして読む。モックは出さない。
extension AppRuntime {
    var planUsageSource: (any PlanUsageSource)? {
        #if DEBUG
        if let mock = DebugMockUsage.source { return mock }
        #endif
        return services.appConfig as? PlanUsageSource
    }

    /// 自分の分と、今のアプリの分を読む(新しければ読まない)。AI の残りは「今日の残り」に入れる(X3)
    func refreshPlanUsage(force: Bool = false) async {
        guard let source = planUsageSource else { return }
        let store = PlanUsageStore.shared
        if let me = await store.refreshMe(source: source, force: force), let today = me.remainingToday(now: .now) {
            RemainingTodayStore.shared.record(today)
        }
        guard !dsl.isVoiceOnly else { return }
        await store.refreshApp(appId: document.appId, source: source, force: force)
    }

    /// 今のアプリの量(作成者のアプリだけ。参加者・まだ分からない時は nil)
    var currentAppUsage: AppUsage? {
        guard !dsl.isVoiceOnly, let usage = PlanUsageStore.shared.app(document.appId), usage.isMine else { return nil }
        return usage
    }
}
