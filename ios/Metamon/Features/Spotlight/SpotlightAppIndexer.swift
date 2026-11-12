import Foundation
import MetamonServices

/// アプリ一覧と Spotlight の索引を合わせる(#1996)。
/// 索引の中身は読めないので、入れたもの(id → 名前と版)を UserDefaults に覚えて差分だけ入れる・消す。
/// このキーはサインアウトの `LocalWipe` でも消える(索引も `removeAll` で消すので揃う)。
@MainActor
final class SpotlightAppIndexer {
    static let indexedKey = "metamon.spotlight.indexedApps"

    let index: any SpotlightIndex
    private let defaults: UserDefaults
    private let icon: (String) -> String?

    init(index: any SpotlightIndex, defaults: UserDefaults = .standard,
         icon: @escaping (String) -> String? = { DocumentCache.shared.load(appId: $0)?.dsl.app.icon }) {
        self.index = index
        self.defaults = defaults
        self.icon = icon
    }

    /// `GET /v1/apps` の結果に合わせる。一覧に出ないもの(archived・未生成)と一覧に無い id は消す。
    /// `/v1/apps` は `users/{uid}/apps` だけを返すので、他の人が持ち主の共有アプリは入らない。
    func sync(_ apps: [AppSummary]) async {
        let visible = AppSummary.visible(apps, current: nil)
        let before = indexed
        var after: [String: String] = [:]
        for app in visible { after[app.id] = Self.signature(app) }
        let gone = before.keys.filter { after[$0] == nil }.sorted()
        // 名前が変わった・作り直した(アイコンが変わり得る)ものだけ入れ直す
        let changed = visible.filter { before[$0.id] != after[$0.id] }
        if !gone.isEmpty { await index.remove(appIds: gone) }
        if !changed.isEmpty {
            await index.index(apps: changed.map { SpotlightAppItem(id: $0.id, name: $0.name, icon: icon($0.id)) })
        }
        indexed = after
    }

    /// 作ったアプリを消した時
    func remove(appId: String) async {
        await index.remove(appIds: [appId])
        var map = indexed
        map[appId] = nil
        indexed = map
    }

    /// ログアウト・アカウント削除
    func removeAll() async {
        await index.removeAll()
        defaults.removeObject(forKey: Self.indexedKey)
    }

    private var indexed: [String: String] {
        get { defaults.dictionary(forKey: Self.indexedKey) as? [String: String] ?? [:] }
        set { defaults.set(newValue, forKey: Self.indexedKey) }
    }

    private static func signature(_ app: AppSummary) -> String { "\(app.currentVersion)|\(app.name)" }
}
