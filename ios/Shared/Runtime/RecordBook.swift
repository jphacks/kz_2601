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
    @ObservationIgnored private let flight = SaveFlight()
    /// shared / public の entity に新しく作る記録の投稿者(#250・#963)。nil を返す entity(private)・未設定は作者を付けない
    @ObservationIgnored var postAuthor: ((String) -> (uid: String?, name: String?)?)?
    /// 端末に書いた(足す・変える・消す・取り消す)entity。Mac で同じアプリを開いた別のウインドウに知らせる(#1853)
    @ObservationIgnored var onLocalWrite: ((String) -> Void)?

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
        onLocalWrite?(entity)
        return .saved
    }

    /// フォームを通さずに 1 件足す。DSL に無い entity には書かない。
    func create(entity: String, data: [String: JSONValue]) async -> Bool {
        guard dsl.entity(entity) != nil else { return false }
        records[entity] = await recordSync.save(appId: appId, entity: entity, recordId: nil, data: data, author: postAuthor?(entity))
        onLocalWrite?(entity)
        return true
    }

    /// `create` と同じ。作った行の id(仮 id `local-…`)を返す。書けなければ nil(aiChat の対の保存・#1326)
    func createId(entity: String, data: [String: JSONValue]) async -> String? {
        guard dsl.entity(entity) != nil else { return nil }
        let written = await recordSync.write(appId: appId, entity: entity, recordId: nil, data: data, author: postAuthor?(entity))
        records[entity] = written.records
        onLocalWrite?(entity)
        return written.recordId
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
        /// `beforeNew` が止めた(iOS: 共有アプリへの初めての投稿で名前を聞く #250)。持ち主が後でやり直す
        case held
    }

    /// 書き込めた 1 件。`afterSave` に渡す
    struct SavedWrite {
        let entity: String
        /// 書いた行の id(新規は仮 id `local-…`)
        let recordId: String
        let payload: [String: JSONValue]
        let isNew: Bool
    }

    /// 持ち主だけの処理を足す所(iOS のウィジェット・ヘルスケア・期限切れ・名前の確認・断られた時の取り消し)。Mac は使わない
    struct SaveHooks {
        /// 新規の書き込みの前。false で書かずに止める(`.held`)
        var beforeNew: () -> Bool = { true }
        /// 書き込み・`markSaved` の後(画面を進める前)
        var afterSave: (SavedWrite) -> Void = { _ in }

        static let none = SaveHooks()
    }

    /// `form` を保存している途中(書き込みの await の間)か。テストが保存の途中に入力する
    func isSaving(_ form: FormDraft) -> Bool { flight.isSaving(form) }

    /// saveRecord の書き込み(前段の判定は `RecordActionSteps.saveTarget`)。
    func save(entity: String, form: FormDraft, recordId: String?,
              author: (uid: String?, name: String?)? = nil, hooks: SaveHooks = .none,
              canEdit: (String, Record) -> Bool) async -> SaveOutcome {
        await Self.save(appId: appId, entity: entity, form: form, recordId: recordId, recordSync: recordSync, flight: flight,
                        author: author ?? (recordId == nil ? postAuthor?(entity) : nil), hooks: hooks, canEdit: canEdit,
                        current: { self.record(entity, $0) },
                        store: { self.records[entity] = $0; self.onLocalWrite?(entity) })
    }

    /// 保存の手順の本体。記録の置き場が違う iOS の `AppRuntime` もこれを使う(#1141)。
    /// 他人の記録 → 新規の前 → 変更なし → 連打 → 書き込み → `markSaved` → `afterSave` の順。
    static func save(appId: String, entity: String, form: FormDraft, recordId: String?,
                     recordSync: RecordSyncEngine, flight: SaveFlight,
                     author: (uid: String?, name: String?)?, hooks: SaveHooks,
                     canEdit: (String, Record) -> Bool,
                     current: (String) -> Record?, store: ([Record]) -> Void) async -> SaveOutcome {
        if let id = recordId, let record = current(id), !canEdit(entity, record) { return .readOnly } // 送信待ちに積まない(#249)
        if recordId == nil, !hooks.beforeNew() { return .held }
        guard form.needsWrite else { return .unchanged }
        // 保存ボタンの連打で同じ内容が 2 件できないように
        guard flight.begin(form) else { return .inFlight }
        defer { flight.end(form) }
        // 楽観的 UI: ローカル DB に即反映して画面を進める。送信は recordSync のキュー
        let payload = form.payload
        let written = await recordSync.write(appId: appId, entity: entity, recordId: recordId, data: payload, author: author)
        store(written.records)
        // 送った値を保存済みとする(保存の間に入れ直した値は未保存のまま)。オフラインなら送信待ち(M10)
        form.markSaved(queuedOffline: !recordSync.isOnline, sent: payload)
        hooks.afterSave(SavedWrite(entity: entity, recordId: written.recordId, payload: payload, isNew: recordId == nil))
        return .saved
    }

    /// 一覧から隠し、取り消せる間は送らない(M11)。
    func deleteUndoably(entity: String, recordId: String, grace: Duration) async -> DeletionTicket {
        let ticket = await recordSync.deleteUndoably(appId: appId, entity: entity, recordId: recordId, grace: grace).ticket
        await reloadLocal(entity)
        onLocalWrite?(entity)
        return ticket
    }

    /// 取り消す。もう送った(時間切れ)なら false。
    @discardableResult
    func undoDelete(_ ticket: DeletionTicket) async -> Bool {
        guard await recordSync.undoDelete(ticket) != nil else { return false }
        await reloadLocal(ticket.entity)
        onLocalWrite?(ticket.entity)
        return true
    }
}

/// 保存中のフォーム(saveRecord の連打で同じ記録を 2 件作らない)。await をまたぐので参照型で持つ
@MainActor
final class SaveFlight {
    private var forms: Set<ObjectIdentifier> = []

    func begin(_ form: FormDraft) -> Bool { forms.insert(ObjectIdentifier(form)).inserted }
    func end(_ form: FormDraft) { forms.remove(ObjectIdentifier(form)) }
    func isSaving(_ form: FormDraft) -> Bool { forms.contains(ObjectIdentifier(form)) }
}
