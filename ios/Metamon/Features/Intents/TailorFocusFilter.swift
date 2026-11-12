import AppIntents
import Foundation
import MetamonServices

// MARK: - 集中モードのフィルタ(#1991)

/// 集中モードのフィルタで選んだ内容。App Group の UserDefaults に JSON で置く(サーバーには送らない)。
struct FocusFilterState: Codable, Equatable {
    /// 一覧に出す生成アプリ。空 = 絞らない
    var appIds: [String]
    /// 通知を止めるか(絞り込みは別 Issue。ここでは保存だけ)
    var silencesNotifications: Bool

    static let storageKey = "metamon.focusFilter"

    /// 何も選んでいない = フィルタを置かないのと同じ
    var isEmpty: Bool { appIds.isEmpty && !silencesNotifications }

    static func load(from defaults: UserDefaults?) -> FocusFilterState? {
        guard let data = defaults?.data(forKey: storageKey) else { return nil }
        return try? JSONDecoder().decode(FocusFilterState.self, from: data)
    }

    /// 空なら消す(集中モードが切れた時もここに来る)。
    static func apply(_ state: FocusFilterState?, to defaults: UserDefaults?) {
        guard let state, !state.isEmpty, let data = try? JSONEncoder().encode(state) else {
            defaults?.removeObject(forKey: storageKey)
            return
        }
        defaults?.set(data, forKey: storageKey)
    }

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

    func perform() async throws -> some IntentResult {
        FocusFilterState.apply(state, to: WidgetSnapshot.defaults)
        return .result()
    }
}
