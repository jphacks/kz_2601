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
    /// `removeAll` で進める。一覧の取得・索引の await を挟む間にサインアウトしたら、前の人の分を入れ直さない(`AppGlanceStore` と同じ)
    private(set) var generation = 0

    init(index: any SpotlightIndex, defaults: UserDefaults = .standard,
         icon: @escaping (String) -> String? = { DocumentCache.shared.load(appId: $0)?.dsl.app.icon }) {
        self.index = index
        self.defaults = defaults
        self.icon = icon
    }

    /// `GET /v1/apps` の結果に合わせる。一覧に出ないもの(archived・未生成)と一覧に無い id は消す。
    /// `/v1/apps` は `users/{uid}/apps` だけを返すので、他の人が持ち主の共有アプリは入らない。
    /// `since`: 一覧を取りに行く前の `generation`。取っている間にサインアウトしていたら何もしない。
    func sync(_ apps: [AppSummary], since: Int? = nil) async {
        let started = since ?? generation
        guard started == generation else { return }
        let visible = AppSummary.visible(apps, current: nil)
        let before = indexed
        var after: [String: String] = [:]
        for app in visible { after[app.id] = Self.signature(app) }
        let gone = before.keys.filter { after[$0] == nil }.sorted()
        // 名前が変わった・作り直した(アイコンが変わり得る)ものだけ入れ直す
        let changed = visible.filter { before[$0.id] != after[$0.id] }
        if !gone.isEmpty { await index.remove(appIds: gone) }
        guard started == generation else { return }
        if !changed.isEmpty {
            let items = changed.map { SpotlightAppItem(id: $0.id, name: $0.name, icon: icon($0.id)) }
            guard await put(items, since: started) else { return }
        }
        indexed = after
    }

    /// 生成で新しく作ったアプリ(一覧を取り直さずに 1 件だけ入れる)。生成の結果なので未生成扱いにはしない。
    func upsert(_ app: AppSummary, icon appIcon: String?) async {
        guard !app.isArchived, !app.name.isEmpty else { return }
        let started = generation
        let signature = Self.signature(app)
        guard indexed[app.id] != signature else { return }
        guard await put([SpotlightAppItem(id: app.id, name: app.name, icon: appIcon ?? icon(app.id))], since: started) else { return }
        var map = indexed
        map[app.id] = signature
        indexed = map
    }

    /// 入れる。入れている間にサインアウトしたら、入ったかもしれない分を消して false
    private func put(_ items: [SpotlightAppItem], since started: Int) async -> Bool {
        await index.index(apps: items)
        guard started == generation else {
            await index.remove(appIds: items.map(\.id))
            return false
        }
        return true
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
        generation += 1
        await index.removeAll()
        defaults.removeObject(forKey: Self.indexedKey)
    }

    private var indexed: [String: String] {
        get { defaults.dictionary(forKey: Self.indexedKey) as? [String: String] ?? [:] }
        set { defaults.set(newValue, forKey: Self.indexedKey) }
    }

    private static func signature(_ app: AppSummary) -> String { "\(app.currentVersion)|\(app.name)" }
}
