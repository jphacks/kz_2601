import SwiftUI
import MetamonServices

/// Pro かどうか(iOS の `RemainingTodayPlan` の Mac 版・#1219)。「今日の残り」(`RemainingTodaySettingsSection`)が読む。
/// 審査用の版は App 内課金を持たない(#450 G1)ので常に無料。
@MainActor
enum RemainingTodayPlan {
    static var isPro: Bool {
        #if APP_STORE_REVIEW
        false
        #else
        MacPro.shared.isPro
        #endif
    }
}

/// 設定の「使っている量」(#317 の Mac 版・#1219)。作ったアプリの数だけ出す
/// (記録・写真はアプリごとなので、iOS と同じく開いているアプリの話。Mac の設定ウインドウはアプリに属さないので出さない)。
/// 量はサーバー(`GET /v1/me/usage`)から 5 分だけキャッシュして読む。サーバーにつながっていない・古いサーバーなら出さない。
struct MacUsageSettingsSection: View {
    var source: (any PlanUsageSource)? = MacServices.shared.appConfig as? PlanUsageSource
    private let store = PlanUsageStore.shared

    var body: some View {
        Group {
            if let apps = store.me?.apps {
                Section {
                    LabeledContent(UsageCopy.rowTitle(.apps)) {
                        Text(UsageCopy.amount(apps))
                            .monospacedDigit()
                            .foregroundStyle(UsageNoticeRules.isNear(apps) ? AnyShapeStyle(Theme.Colors.warning) : AnyShapeStyle(.secondary))
                    }
                    .accessibilityElement(children: .ignore)
                    .accessibilityLabel(UsageCopy.rowTitle(.apps))
                    .accessibilityValue(UsageCopy.accessibilityAmount(apps))
                    .accessibilityIdentifier("settings.usage.apps")
                } header: {
                    Text(UsageCopy.sectionTitle)
                } footer: {
                    Text(UsageCopy.footer(isPro: RemainingTodayPlan.isPro || store.me?.isPro == true))
                }
            }
        }
        .task { await Self.refresh(source: source) }
    }

    /// 自分の分を読む(新しければ読まない)。AI の残りは「今日の残り」にも入れる(iOS の `refreshPlanUsage` と同じ)
    static func refresh(source: (any PlanUsageSource)?, force: Bool = false) async {
        guard let source else { return }
        let store = PlanUsageStore.shared
        if let me = await store.refreshMe(source: source, force: force), let at = store.meObservedAt,
           let today = me.remainingToday(observedAt: at) {
            RemainingTodayStore.shared.record(today)
        }
    }
}
