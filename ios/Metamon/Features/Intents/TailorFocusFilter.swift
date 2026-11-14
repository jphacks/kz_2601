import AppIntents
import Foundation
import MetamonServices
import WidgetKit

// MARK: - 集中モードのフィルタ(#1991)

/// 型と保存・出すかの判定は MetamonWidget/Shared/FocusFilterState.swift(ウィジェットも読む・#1992)。
extension FocusFilterState {
    /// 選んだアプリだけを元の並びのまま残す。選んだアプリが 1 つも無い(消した等)時は絞らない(空の一覧にしない)。
    func filter(_ apps: [AppSummary]) -> [AppSummary] {
        guard !appIds.isEmpty else { return apps }
        let picked = Set(appIds)
        let kept = apps.filter { picked.contains($0.id) }
        return kept.isEmpty ? apps : kept
    }
}

/// 設定アプリ › 集中モード › フィルタに出る「Tailor It」。
struct TailorFocusFilter: SetFocusFilterIntent {
    static var title: LocalizedStringResource = "Tailor It のフィルタ"
    #if APP_STORE_REVIEW // 審査用の版は記録帳と言う(#247)
    static var description: IntentDescription? = IntentDescription("集中モードの間、選んだ記録帳だけを一覧に出します。")

    @Parameter(title: "出す記録帳", default: [])
    var apps: [TailorAppEntity]
    #else
    static var description: IntentDescription? = IntentDescription("集中モードの間、選んだアプリだけを変身一覧に出します。")

    @Parameter(title: "出すアプリ", default: [])
    var apps: [TailorAppEntity]
    #endif

    @Parameter(title: "通知を止める", default: false)
    var silencesNotifications: Bool

    var displayRepresentation: DisplayRepresentation {
        let title = apps.isEmpty
            ? LocalizedStringResource("絞らない")
            : LocalizedStringResource("\(apps.map(\.name).formatted(.list(type: .and)))だけ")
        return DisplayRepresentation(title: title, subtitle: silencesNotifications ? "通知を止める" : nil)
    }

    var state: FocusFilterState { FocusFilterState(appIds: apps.map(\.id), silencesNotifications: silencesNotifications) }

    /// 「通知を止める」の間、選んでいない生成アプリの通知を OS が出さない(通知の `filterCriteria` = 生成アプリの id・#1992)。
    var appContext: FocusFilterAppContext {
        FocusFilterAppContext(notificationFilterPredicate: state.notificationFilterPredicate)
    }

    func perform() async throws -> some IntentResult {
        FocusFilterState.apply(state, to: WidgetSnapshot.defaults)
        WidgetCenter.shared.reloadAllTimelines() // 選んでいないアプリの数字を「集中モード中」に替える・切れたら戻す(#1992)
        return .result()
    }
}
