import Foundation
import MetamonServices

/// 最近開いた生成アプリの順(#2232)。ホーム画面のアイコンの長押しメニューに使う。
/// 端末の UserDefaults に appId だけを持つ(ログアウトの `LocalWipe` で消える)。
struct RecentApps {
    static let key = "metamon.recentApps"
    /// 持っておく数(消したアプリを除いても 3 つ残るよう多めに)
    static let maxStored = 12

    var defaults: UserDefaults = .standard

    /// 新しい順
    var ids: [String] { defaults.stringArray(forKey: Self.key) ?? [] }

    func noteOpened(_ appId: String) {
        guard !appId.isEmpty else { return }
        defaults.set(Self.noting(appId, in: ids), forKey: Self.key)
    }

    func forget(_ appId: String) {
        defaults.set(ids.filter { $0 != appId }, forKey: Self.key)
    }

    /// 先頭に入れ、重なりを除き、上限で切る。
    static func noting(_ appId: String, in ids: [String]) -> [String] {
        Array(([appId] + ids.filter { $0 != appId }).prefix(maxStored))
    }
}

/// 長押しメニューの項目(並びを決める所・単体テストの対象)。
enum QuickActionItem: Equatable {
    case app(id: String, name: String)
    /// 声で頼む(音声入力のシートを開く)
    case talk

    /// 全部で 4 つまで(OS の既定の表示数)。アプリは 3 つまで。
    static let maxItems = 4
    static let maxApps = 3

    /// 最近開いた順のうち、一覧にある(消していない・空でない)アプリを 3 つまで + 「声で頼む」。
    /// 一度も開いていないアプリは足さない(作っただけのアプリが並ばないように)。
    static func items(recent: [String], apps: [AppSummary]) -> [QuickActionItem] {
        let byId = Dictionary(AppSummary.visible(apps, current: nil).map { ($0.id, $0) }, uniquingKeysWith: { a, _ in a })
        var seen = Set<String>()
        var result: [QuickActionItem] = []
        for id in recent where result.count < maxApps {
            guard seen.insert(id).inserted, let app = byId[id] else { continue }
            let name = app.name.trimmingCharacters(in: .whitespacesAndNewlines)
            guard !name.isEmpty else { continue }
            result.append(.app(id: id, name: name))
        }
        result.append(.talk)
        return Array(result.prefix(maxItems))
    }
}
