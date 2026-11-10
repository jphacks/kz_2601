import Foundation
import MetamonDSL
import MetamonServices

/// 端末だけに置く「作ったアプリ」の一覧(#969・Mac)。サーバー(#930)が入るまでの置き場で、iOS は使わない。
/// 置き方は iOS のキャッシュと同じ: 一覧(名前・並び)は `AppListCache`(apps.json)、DSL は `DocumentCache`(appId ごとの封筒)。
/// 一覧の並びは配列の順(先頭が上)。名前の検査と並べ替えは iOS と同じ `AppListEdit`。
struct LocalAppLibrary {
    struct Entry {
        var summary: AppSummary
        var document: AppDocument
        var envelope: Data
    }

    static let currentAppKey = "metamon.local.currentAppId"

    let list: AppListCache
    let documents: DocumentCache
    let defaults: UserDefaults

    static var standard: LocalAppLibrary {
        LocalAppLibrary(list: .shared, documents: .shared, defaults: .standard)
    }

    /// 保存した一覧を読む。DSL が読めないアプリは外す(一覧だけ残っても開けないため)
    func load() -> [Entry] {
        var seen = Set<String>()
        return list.load().compactMap { summary in
            guard seen.insert(summary.id).inserted, let envelope = documents.loadData(appId: summary.id),
                  var document = try? AppDocument.decode(envelope) else { return nil }
            document.appId = summary.id
            return Entry(summary: summary, document: document, envelope: envelope)
        }
    }

    /// 一覧(名前・並び)を置く
    func save(_ summaries: [AppSummary]) { list.store(summaries) }

    func store(appId: String, envelope: Data) { documents.store(appId: appId, envelope: envelope) }

    func remove(appId: String) {
        documents.remove(appId: appId)
        if currentAppId == appId { currentAppId = nil }
    }

    /// 最後に開いていたアプリ(起動時に開き直す)
    var currentAppId: String? {
        get { defaults.string(forKey: Self.currentAppKey) }
        nonmutating set { defaults.set(newValue, forKey: Self.currentAppKey) }
    }

    /// 一覧に置く 1 行。`sortIndex` は並びの位置(1 が先頭)
    static func summaries(_ entries: [(id: String, name: String, version: Int)]) -> [AppSummary] {
        entries.enumerated().map { i, e in
            AppSummary(id: e.id, name: e.name, currentVersion: e.version, sortIndex: i + 1)
        }
    }

    /// 開き直すアプリ: 前に開いていたアプリが一覧にあればそれ、無ければ nil(何も選ばない)
    static func reopen(_ saved: String?, among ids: [String]) -> String? {
        guard let saved, ids.contains(saved) else { return nil }
        return saved
    }

    /// `id` を消した後に選ぶアプリ: 一覧で下にあるアプリ、無ければ上のアプリ、どちらも無ければ nil
    static func neighbor(deleting id: String, in ids: [String]) -> String? {
        guard let i = ids.firstIndex(of: id) else { return nil }
        if i + 1 < ids.count { return ids[i + 1] }
        return i > 0 ? ids[i - 1] : nil
    }
}
