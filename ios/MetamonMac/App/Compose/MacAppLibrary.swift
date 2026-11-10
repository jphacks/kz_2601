import Foundation
import Observation
import MetamonDSL
import MetamonServices

/// 会話で作った・直したアプリの DSL と、その版の「元に戻す / やり直す」(#967・#968)、作ったアプリの一覧の整理(#969)。
/// - 作ったアプリは端末に残す(`LocalAppLibrary`: iOS のキャッシュと同じ置き方)。サーバー(#930)が入るまでは端末だけ
/// - 同梱の例を会話で直した分と版の履歴は、今まで通りウインドウを閉じる(アプリを終える)と消える
/// - 版の nil は「同梱の例のまま(会話で直す前)」。同梱の例を直して戻すと、元の例を表示し直す
/// ウインドウをまたいで 1 つだけ持つ(`MetamonMacApp` が作って渡す)。
@MainActor
@Observable
final class MacAppLibrary {
    struct Version {
        var document: AppDocument
        /// 端末に残す形(封筒)。無い版(古い結果など)は残さない
        var envelope: Data?
    }

    private(set) var generated: [String: AppDocument] = [:]
    /// 作ったアプリの id(一覧の並び・上から)
    private(set) var createdIds: [String] = []
    private var envelopes: [String: Data] = [:]
    private var histories: [String: LocalVersionHistory<Version?>] = [:]
    @ObservationIgnored private let store: LocalAppLibrary?

    /// `store` が nil なら端末に残さない(プレビュー・テスト用)
    init(store: LocalAppLibrary? = .standard) {
        self.store = store
        for entry in store?.load() ?? [] {
            generated[entry.summary.id] = entry.document
            envelopes[entry.summary.id] = entry.envelope
            createdIds.append(entry.summary.id)
        }
    }

    func isCreated(_ appId: String) -> Bool { createdIds.contains(appId) }

    /// 最後に開いていたアプリ(起動時に開き直す)
    var lastAppId: String? {
        get { store?.currentAppId }
        set { store?.currentAppId = newValue }
    }

    /// 会話の結果を当てる。作った直後の版には戻り先が無い(iOS も作った直後は「元に戻す」を出さない)。作ったアプリは一覧の先頭に足す
    func apply(_ document: AppDocument, envelope: Data? = nil, created: Bool) {
        let id = document.appId
        if !(created && generated[id] == nil) {
            histories[id, default: .init()].record(previous: current(id))
        }
        if created, !createdIds.contains(id) { createdIds.insert(id, at: 0) }
        set(id, Version(document: document, envelope: envelope))
    }

    func canUndo(_ appId: String) -> Bool { histories[appId]?.canUndo ?? false }
    func canRedo(_ appId: String) -> Bool { histories[appId]?.canRedo ?? false }

    /// 「元に戻す」。戻せたら true
    @discardableResult
    func undo(_ appId: String) -> Bool {
        guard var history = histories[appId], let previous = history.undo(from: current(appId)) else { return false }
        histories[appId] = history
        set(appId, previous)
        return true
    }

    /// 「やり直す」。やり直せたら true
    @discardableResult
    func redo(_ appId: String) -> Bool {
        guard var history = histories[appId], let next = history.redo(from: current(appId)) else { return false }
        histories[appId] = history
        set(appId, next)
        return true
    }

    // MARK: - 一覧の整理(#969・iOS の AppRuntime+AppList / +Delete)

    /// 名前を変える(作ったアプリだけ)。iOS と同じく版の履歴に積む(「元に戻す」で前の名前に戻せる)。
    /// 中身は変わらないので版の番号は上げない(中央を作り直さず、開いている記録をそのまま残す)。名前が空 / 長すぎなら false
    @discardableResult
    func rename(_ appId: String, to raw: String) -> Bool {
        guard let name = AppListEdit.normalizedName(raw) else { return false }
        guard isCreated(appId), var document = generated[appId], document.dsl.app.name != name else { return true }
        histories[appId, default: .init()].record(previous: current(appId))
        document.dsl.app.name = name
        set(appId, Version(document: document, envelope: envelopes[appId].flatMap { DocumentEnvelope.renamed($0, to: name) }))
        return true
    }

    /// 削除する(作ったアプリだけ)。端末の DSL と版の履歴も消す
    func delete(_ appId: String) {
        guard let index = createdIds.firstIndex(of: appId) else { return }
        createdIds.remove(at: index)
        generated[appId] = nil
        envelopes[appId] = nil
        histories[appId] = nil
        store?.remove(appId: appId)
        saveList()
    }

    /// 作ったアプリの `index` 番目に動かす(右クリックの「上へ / 下へ」)
    func move(_ appId: String, to index: Int) {
        let next = AppListEdit.moved(createdIds, id: appId, to: index)
        guard next != createdIds else { return }
        createdIds = next
        saveList()
    }

    /// ドラッグで並べ替え(`List` の `onMove`)
    func move(fromOffsets source: IndexSet, toOffset destination: Int) {
        var next = createdIds
        next.move(fromOffsets: source, toOffset: destination)
        guard next != createdIds else { return }
        createdIds = next
        saveList()
    }

    // MARK: - 内部

    private func current(_ appId: String) -> Version? {
        generated[appId].map { Version(document: $0, envelope: envelopes[appId]) }
    }

    /// 今の版を差し替える。作ったアプリは端末にも残す
    private func set(_ appId: String, _ version: Version?) {
        generated[appId] = version?.document
        envelopes[appId] = version?.envelope
        guard isCreated(appId) else { return }
        if let envelope = version?.envelope { store?.store(appId: appId, envelope: envelope) }
        saveList() // 名前・版が変わる
    }

    private func saveList() {
        guard let store else { return }
        store.save(LocalAppLibrary.summaries(createdIds.compactMap { id in
            generated[id].map { (id, $0.dsl.app.name, $0.version) }
        }))
    }
}
