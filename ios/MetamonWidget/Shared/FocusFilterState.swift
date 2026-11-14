import Foundation

/// 集中モードのフィルタで選んだ内容(#1991)。App Group の UserDefaults に JSON で置く(サーバーには送らない)。
/// 本体(`TailorFocusFilter`)が書き、本体の一覧とウィジェット(#1992)が読む。本体と MetamonWidget にコンパイルされる(project.yml)。
struct FocusFilterState: Codable, Equatable {
    /// 一覧に出す生成アプリ。空 = 絞らない
    var appIds: [String]
    /// 選んでいない生成アプリの通知を止めるか(#1992)
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

    // MARK: - 出すか(#1992)

    /// その生成アプリのローカル通知を出すか。「通知を止める」の時だけ、選んだアプリ以外を止める(何も選んでいなければ全部止める)。
    func allowsNotification(appId: String) -> Bool {
        !silencesNotifications || appIds.contains(appId)
    }

    /// OS に渡す絞り込み(`FocusFilterAppContext.notificationFilterPredicate`)。通知の `filterCriteria`(= 生成アプリの id)に当てる。
    /// 判定は `allowsNotification` と同じ。止めない時は nil(OS は絞らない)。
    var notificationFilterPredicate: NSPredicate? {
        silencesNotifications ? NSPredicate(format: "SELF IN %@", appIds) : nil
    }

    /// ウィジェットにその生成アプリの数字を出すか。一覧を絞っている時だけ、選んだアプリ以外は出さない(どのアプリか分からない古いデータも出さない)。
    func allowsWidget(appId: String?) -> Bool {
        guard !appIds.isEmpty else { return true }
        guard let appId else { return false }
        return appIds.contains(appId)
    }

    /// ウィジェットの数字を「集中モード中」に替えるか。数字が無い(まだアプリが無い)・フィルタが無い時は替えない(今まで通り)。
    static func pausesWidget(_ snapshot: WidgetSnapshot?, focus: FocusFilterState?) -> Bool {
        guard let snapshot, let focus else { return false }
        return !focus.allowsWidget(appId: snapshot.appId)
    }
}
