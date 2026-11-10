import Observation
import MetamonDSL
import MetamonRenderer
import MetamonServices

/// 表示中のアプリの記録(ローカル DB → 同期キュー)の読み書き。UIKit・AppKit に依存しないので Mac のランタイムが持ち、
/// iOS の単体テストからも確かめる(#1338)。一言・画面の移動・取り消しのトーストは持ち主(ランタイム)が出す。
@MainActor
@Observable
final class RecordBook {
    let appId: String
    @ObservationIgnored let dsl: AppDSL
    @ObservationIgnored let recordSync: RecordSyncEngine
    private(set) var loading: Set<String> = []
    private(set) var failed: Set<String> = []
    private(set) var records: [String: [Record]] = [:]
    @ObservationIgnored private var savingForms: Set<ObjectIdentifier> = []

    init(appId: String, dsl: AppDSL, recordSync: RecordSyncEngine) {
        self.appId = appId
        self.dsl = dsl
        self.recordSync = recordSync
    }

    func records(for entity: String) -> [Record] { records[entity] ?? [] }

    func record(_ entity: String, _ id: String) -> Record? {
        records[entity]?.first { $0.id == id }
    }

    func load(entities: Set<String>, force: Bool) async {
        for e in entities where force || records[e] == nil {
            await refresh(e)
        }
    }

    /// ローカル先出し → appconfig の全件で置換。取れずローカルにも無ければ「読み込めませんでした」。
    func refresh(_ entity: String) async {
        if records[entity] == nil {
            let cached = await recordSync.cached(appId: appId, entity: entity)
            if !cached.isEmpty { records[entity] = cached }
        }
        loading.insert(entity)
        defer { loading.remove(entity) }
        do {
            records[entity] = try await recordSync.pull(appId: appId, entity: entity)
            failed.remove(entity)
        } catch {
            if records[entity]?.isEmpty ?? true {
                records[entity] = []
                failed.insert(entity)
            }
            print("[Metamon] records pull failed: entity=\(entity) \(error)")
        }
    }

    func reloadLocal(_ entity: String) async {
        records[entity] = await recordSync.cached(appId: appId, entity: entity)
    }

    enum UpdateOutcome: Equatable {
        case saved
        /// 記録が無い(消えた・読み込む前)
        case missing
        /// 他人の記録。持ち主が `notifyReadOnly` を出す
        case readOnly
    }

    /// 1 件の一部の項目を書く(一覧のチェックなど)。書けなければ書かない(楽観的に描いた側が戻す)。
    func update(entity: String, recordId: String, data: [String: JSONValue],
                canEdit: (String, Record) -> Bool) async -> UpdateOutcome {
        guard let current = record(entity, recordId) else { return .missing }
        guard canEdit(entity, current) else { return .readOnly }
        records[entity] = await recordSync.save(appId: appId, entity: entity, recordId: recordId, data: data)
        return .saved
    }

    /// フォームを通さずに 1 件足す。DSL に無い entity には書かない。
    func create(entity: String, data: [String: JSONValue]) async -> Bool {
        guard dsl.entity(entity) != nil else { return false }
        records[entity] = await recordSync.save(appId: appId, entity: entity, recordId: nil, data: data)
        return true
    }

    enum SaveOutcome: Equatable {
        /// 書いた。画面を進める
        case saved
        /// 既存の記録を何も変えずに保存(M10)。書かずに画面だけ進める
        case unchanged
        /// 同じフォームの保存の途中(連打)。何もしない
        case inFlight
        /// 他人の記録。持ち主が `notifyReadOnly` を出す
        case readOnly
    }

    /// saveRecord の書き込み(前段の判定は `RecordActionSteps.saveTarget`)。
    func save(entity: String, form: FormDraft, recordId: String?,
              canEdit: (String, Record) -> Bool) async -> SaveOutcome {
        if let id = recordId, let current = record(entity, id), !canEdit(entity, current) { return .readOnly }
        guard form.needsWrite else { return .unchanged }
        // 保存ボタンの連打で同じ内容が 2 件できないように
        guard savingForms.insert(ObjectIdentifier(form)).inserted else { return .inFlight }
        defer { savingForms.remove(ObjectIdentifier(form)) }
        let written = await recordSync.write(appId: appId, entity: entity, recordId: recordId, data: form.payload)
        records[entity] = written.records
        form.markSaved(queuedOffline: !recordSync.isOnline)
        return .saved
    }

    /// 一覧から隠し、取り消せる間は送らない(M11)。
    func deleteUndoably(entity: String, recordId: String, grace: Duration) async -> DeletionTicket {
        let ticket = await recordSync.deleteUndoably(appId: appId, entity: entity, recordId: recordId, grace: grace).ticket
        await reloadLocal(entity)
        return ticket
    }

    /// 取り消す。もう送った(時間切れ)なら false。
    @discardableResult
    func undoDelete(_ ticket: DeletionTicket) async -> Bool {
        guard await recordSync.undoDelete(ticket) != nil else { return false }
        await reloadLocal(ticket.entity)
        return true
    }
}
