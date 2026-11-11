import Foundation
import MetamonDSL
import MetamonServices

/// 端末だけに置く「作ったアプリ」の一覧(#969・Mac)。サーバー(#930)が入るまでの置き場で、iOS は使わない。
/// 置き方は iOS のキャッシュと同じ: 一覧(名前・並び)は `AppListCache`、DSL は `DocumentCache`(appId ごとの封筒)。
/// 一覧は `metamon-dsl/mac-library/apps.json`(#1057)。`metamon-dsl/apps.json`(`AppListCache.shared`)は iOS と同じく
/// サーバーの一覧(`GET /v1/apps`)のキャッシュに使うので分ける。前の版の一覧は初回に移す(`legacyList`)。
/// 一覧の並びは配列の順(先頭が上)。名前の検査と並べ替えは iOS と同じ `AppListEdit`。
/// 会話で直した同梱の例(#1283)も DSL を同じ `DocumentCache` に置き、直した例の id を `editedExamplesKey` に持つ。
struct LocalAppLibrary {
    struct Entry {
        var summary: AppSummary
        var document: AppDocument
        var envelope: Data
    }

    /// 一覧に置く名前・版。端末に残した DSL と同じものだけを持つ
    struct Row: Equatable {
        var name: String
        var version: Int
    }

    static let currentAppKey = "metamon.local.currentAppId"
    static let editedExamplesKey = "metamon.local.editedExampleIds"

    let list: AppListCache
    let documents: DocumentCache
    let defaults: UserDefaults
    /// 前の版が一覧を置いていた所(`AppListCache.shared`)。`list` がまだ無い時だけ 1 回読んで移す
    var legacyList: AppListCache?

    /// 端末の一覧の置き場(`DocumentCache` と同じフォルダの下・サインアウトの wipe で一緒に消える)
    static let standardList: AppListCache = {
        let base = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first
            ?? FileManager.default.temporaryDirectory
        return AppListCache(directory: base.appendingPathComponent("metamon-dsl/mac-library", isDirectory: true))
    }()

    static var standard: LocalAppLibrary {
        LocalAppLibrary(list: standardList, documents: .shared, defaults: .standard, legacyList: .shared)
    }

    /// 保存した一覧を読む。DSL が読めないアプリは外す(一覧だけ残っても開けないため)
    func load() -> [Entry] {
        var seen = Set<String>()
        return loadSummaries().compactMap { summary in
            guard seen.insert(summary.id).inserted, let envelope = documents.loadData(appId: summary.id),
                  var document = try? AppDocument.decode(envelope) else { return nil }
            document.appId = summary.id
            return Entry(summary: summary, document: document, envelope: envelope)
        }
    }

    /// `list` が無ければ前の置き場から移す(無ければ空で作り、以後はサーバーの一覧を端末の一覧と取り違えない)
    private func loadSummaries() -> [AppSummary] {
        guard !list.exists else { return list.load() }
        let migrated = legacyList?.load() ?? []
        list.store(migrated)
        return migrated
    }

    /// 一覧(名前・並び)を置く
    func save(_ summaries: [AppSummary]) { list.store(summaries) }

    func store(appId: String, envelope: Data) { documents.store(appId: appId, envelope: envelope) }

    func remove(appId: String) {
        documents.remove(appId: appId)
        if currentAppId == appId { currentAppId = nil }
    }

    // MARK: - 会話で直した同梱の例(#1283)

    /// 直した同梱の例(id → DSL)。DSL が読めない例は外す(元の例を出す)
    func loadEditedExamples() -> [String: (document: AppDocument, envelope: Data)] {
        var result: [String: (document: AppDocument, envelope: Data)] = [:]
        for id in editedExampleIds {
            guard let envelope = documents.loadData(appId: id), var document = try? AppDocument.decode(envelope) else { continue }
            document.appId = id
            result[id] = (document, envelope)
        }
        return result
    }

    /// 直した同梱の例を置く
    func storeExample(appId: String, envelope: Data) {
        documents.store(appId: appId, envelope: envelope)
        if !editedExampleIds.contains(appId) { editedExampleIds.append(appId) }
    }

    /// 直した同梱の例を消す(元の例に戻す)。開いていたアプリの記憶はそのまま(元の例を開き直す)
    func removeExample(appId: String) {
        documents.remove(appId: appId)
        editedExampleIds.removeAll { $0 == appId }
    }

    private var editedExampleIds: [String] {
        get { defaults.stringArray(forKey: Self.editedExamplesKey) ?? [] }
        nonmutating set { defaults.set(newValue, forKey: Self.editedExamplesKey) }
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

    /// 版を当てた後に一覧へ置く行。封筒(端末に残す DSL)が無い版は DSL を残せないので、行も前のまま。
    /// 一覧だけ進めると、次に開いた時に古い DSL へ新しい名前・版が付くため
    static func row(after document: AppDocument?, envelope: Data?, previous: Row?) -> Row? {
        guard let document, envelope != nil else { return previous }
        return Row(name: document.dsl.app.name, version: document.version)
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
