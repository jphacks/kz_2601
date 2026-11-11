import Foundation
import Observation
import MetamonDSL
import MetamonServices

/// 会話で作った・直したアプリの DSL と、その版の「元に戻す / やり直す」(#967・#968)、作ったアプリの一覧の整理(#969)・複製(#1285)。
/// - 作ったアプリは端末に残す(`LocalAppLibrary`: iOS のキャッシュと同じ置き方)。サーバー(#930)が入るまでは端末だけ
/// - 同梱の例を会話で直した分も端末に残す(#1283)。右クリックの「元の例に戻す」で消す。版の履歴はアプリを終えると消える
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
    /// 一覧に残す名前・版。端末の DSL と揃える(封筒の無い版を当てても進めない)
    private var savedRows: [String: LocalAppLibrary.Row] = [:]
    private var histories: [String: LocalVersionHistory<Version?>] = [:]
    @ObservationIgnored private let store: LocalAppLibrary?

    /// `store` が nil なら端末に残さない(プレビュー・テスト用)
    init(store: LocalAppLibrary? = .standard) {
        self.store = store
        for entry in store?.load() ?? [] {
            generated[entry.summary.id] = entry.document
            envelopes[entry.summary.id] = entry.envelope
            savedRows[entry.summary.id] = .init(name: entry.document.dsl.app.name, version: entry.document.version)
            createdIds.append(entry.summary.id)
        }
        for (id, edited) in store?.loadEditedExamples() ?? [:] where generated[id] == nil {
            generated[id] = edited.document
            envelopes[id] = edited.envelope
        }
    }

    /// 会話で直した同梱の例(右クリックで「元の例に戻す」を出す)
    func isEditedExample(_ appId: String) -> Bool { !isCreated(appId) && generated[appId] != nil }

    /// 同梱の例を元に戻す(#1283)。端末に残した DSL と版の履歴を消す(確認はサイドバーで済んでいる)
    func resetExample(_ appId: String) {
        guard isEditedExample(appId) else { return }
        generated[appId] = nil
        envelopes[appId] = nil
        histories[appId] = nil
        store?.removeExample(appId: appId)
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
        savedRows[appId] = nil
        histories[appId] = nil
        store?.remove(appId: appId)
        saveList()
    }

    /// 複製する(#1285)。名前の付け方は iOS の `duplicateApp` と同じ(「○○のコピー」)。画面の定義だけを写し、記録は写さない。
    /// 作ったアプリは元のすぐ下、同梱の例は作ったアプリの先頭に置く。切り替えはしない(iOS と同じ)。複製したアプリの名前を返す
    @discardableResult
    func duplicate(_ appId: String, bundled: SampleApp? = nil) -> String? {
        let source = envelopes[appId] ?? bundled?.url.flatMap { url in
            (try? Data(contentsOf: url)).flatMap { DocumentEnvelope.make(appId: appId, version: 1, dsl: $0) }
        }
        guard let source, let original = generated[appId]?.dsl.app.name ?? bundled?.name else { return nil }
        let name = AppListEdit.copyName(of: original) { L10n.tr("\($0)のコピー") }
        let id = "mac-\(UUID().uuidString.prefix(8).lowercased())" // 会話で作るアプリと同じ形(MacComposeSession)
        guard let envelope = DocumentEnvelope.duplicated(source, appId: id, name: name),
              var document = try? AppDocument.decode(envelope) else { return nil }
        document.appId = id
        createdIds.insert(id, at: createdIds.firstIndex(of: appId).map { $0 + 1 } ?? 0)
        set(id, Version(document: document, envelope: envelope))
        return name
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

    /// 今の版を差し替える。作ったアプリは端末にも残す。封筒の無い版は DSL も一覧も前のまま(古い DSL に新しい名前を付けない)
    private func set(_ appId: String, _ version: Version?) {
        generated[appId] = version?.document
        envelopes[appId] = version?.envelope
        guard isCreated(appId) else {
            // 同梱の例(#1283): 直した版は端末に残し、元の例まで戻したら消す
            if version == nil { store?.removeExample(appId: appId) }
            else if let envelope = version?.envelope { store?.storeExample(appId: appId, envelope: envelope) }
            return
        }
        guard let envelope = version?.envelope else { return }
        store?.store(appId: appId, envelope: envelope)
        savedRows[appId] = LocalAppLibrary.row(after: version?.document, envelope: envelope, previous: savedRows[appId])
        saveList() // 名前・版が変わる
    }

    private func saveList() {
        guard let store else { return }
        store.save(LocalAppLibrary.summaries(createdIds.compactMap { id in
            savedRows[id].map { (id, $0.name, $0.version) }
        }))
    }
}
