import Foundation
import MetamonDSL

/// ローカル行の未送信状態。`none` 以外は `SyncQueue` に対応する操作が積まれている。
public enum PendingOp: String, Codable, Sendable {
    case none, create, update, delete
}

/// レコードのローカル DB 1 行(Issue #48 の汎用スキーマ `{appId, entity, recordId, data, updatedAt, pendingOp, clientId}`)。
/// `data` は DSL の Field に関係なく丸ごと保持する(entity/field が DSL から消えても行は残し、表示側が無視するだけ)。
public struct LocalRecord: Codable, Sendable, Equatable, Identifiable {
    public var appId: String
    public var entity: String
    public var recordId: String
    public var data: [String: JSONValue]
    public var createdAt: String?
    public var updatedAt: String?
    public var pendingOp: PendingOp
    /// 作成の冪等キー(= 未送信 create の仮 id の元)。サーバー採番後は参照用に残す。
    public var clientId: String?

    public var id: String { recordId }

    public init(appId: String, entity: String, recordId: String, data: [String: JSONValue],
                createdAt: String? = nil, updatedAt: String? = nil, pendingOp: PendingOp = .none, clientId: String? = nil) {
        self.appId = appId
        self.entity = entity
        self.recordId = recordId
        self.data = data
        self.createdAt = createdAt
        self.updatedAt = updatedAt
        self.pendingOp = pendingOp
        self.clientId = clientId
    }

    public init(appId: String, entity: String, server record: Record) {
        self.init(appId: appId, entity: entity, recordId: record.id, data: record.data,
                  createdAt: record.createdAt, updatedAt: record.updatedAt)
    }

    /// Renderer に渡す形。
    public var record: Record { Record(id: recordId, data: data, createdAt: createdAt, updatedAt: updatedAt) }

    /// 未送信 create の仮 id(`local-<clientId>`)。サーバー採番後に差し替わる。
    public static func temporaryId(clientId: String) -> String { "local-\(clientId)" }
    public var isTemporary: Bool { recordId.hasPrefix("local-") }
}

/// レコードのローカル DB。
///
/// 選定: SwiftData / SQLite ではなく **FileManager + JSON(appId/entity ごとに 1 ファイル)**。
/// - 1 entity ≤ 1000 件(`HTTPAppConfigClient.maxPages`)・読みは entity 単位の全件なので、ファイル=索引(appId, entity)で足りる
/// - `DocumentCache`(DSL 封筒)と同じ流儀で、スキーマ移行・ModelContainer・`@Model` の strict concurrency 対応を抱えない
/// - macOS の `swift test` でもそのまま動く(SwiftData の in-memory コンテナ無しで CRUD を検証できる)
/// 行数が万単位になったら `entity.json` を SQLite(GRDB 無しの `sqlite3`)に置き換える。I/F は変えない。
public actor RecordStore {
    private let directory: URL
    /// "appId/entity" → 行(メモリキャッシュ。ファイルは書き込みのたびに atomic 更新)
    private var tables: [String: [LocalRecord]] = [:]

    public init(directory: URL? = nil) {
        let base = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first
            ?? FileManager.default.temporaryDirectory
        self.directory = directory ?? base.appendingPathComponent("metamon-records", isDirectory: true)
    }

    // MARK: - Read

    /// 表示用: 削除待ちを除き、createdAt 降順(サーバー既定と同じ)。未送信 create は createdAt=今なので先頭に来る。
    public func records(appId: String, entity: String) -> [Record] {
        rows(appId: appId, entity: entity)
            .filter { $0.pendingOp != .delete }
            .map(\.record)
    }

    public func rows(appId: String, entity: String) -> [LocalRecord] {
        table(appId, entity).sorted { ($0.createdAt ?? "") > ($1.createdAt ?? "") }
    }

    public func row(appId: String, entity: String, recordId: String) -> LocalRecord? {
        table(appId, entity).first { $0.recordId == recordId }
    }

    /// 同期済み行の最大 updatedAt(差分取得の基準。サーバーに `updatedAfter` が無い間は未使用)。
    public func lastUpdatedAt(appId: String, entity: String) -> String? {
        table(appId, entity).filter { $0.pendingOp == .none }.compactMap(\.updatedAt).max()
    }

    /// 未送信の行数(appId 単位。設定シートの「未送信 N 件」)。
    public func pendingCount(appId: String) -> Int {
        tables.filter { $0.key.hasPrefix(appId + "/") }.values.joined().filter { $0.pendingOp != .none }.count
    }

    // MARK: - Write

    public func upsert(_ row: LocalRecord) {
        var t = table(row.appId, row.entity)
        if let i = t.firstIndex(where: { $0.recordId == row.recordId }) { t[i] = row } else { t.append(row) }
        save(row.appId, row.entity, t)
    }

    public func remove(appId: String, entity: String, recordId: String) {
        var t = table(appId, entity)
        t.removeAll { $0.recordId == recordId }
        save(appId, entity, t)
    }

    /// サーバーの全件で置き換える(サーバーが正)。未送信(`pendingOp != none`)の行だけは残す。
    /// - create 待ちの仮行: そのまま残す
    /// - update/delete 待ちの行: ローカルの内容を優先(送信後に次の pull でサーバー版になる)
    @discardableResult
    public func replace(appId: String, entity: String, with server: [Record]) -> [Record] {
        let pending = table(appId, entity).filter { $0.pendingOp != .none }
        var t = server.map { LocalRecord(appId: appId, entity: entity, server: $0) }
        for p in pending {
            if let i = t.firstIndex(where: { $0.recordId == p.recordId }) { t[i] = p } else { t.append(p) }
        }
        save(appId, entity, t)
        return records(appId: appId, entity: entity)
    }

    /// 同期完了: 仮 id の行をサーバーのレコードに差し替える(create)/ pendingOp を落とす(update)。
    public func markSynced(appId: String, entity: String, localId: String, server: Record) {
        var t = table(appId, entity)
        t.removeAll { $0.recordId == localId }
        t.removeAll { $0.recordId == server.id }
        var row = LocalRecord(appId: appId, entity: entity, server: server)
        row.clientId = localId.hasPrefix("local-") ? String(localId.dropFirst("local-".count)) : nil
        t.append(row)
        save(appId, entity, t)
    }

    /// 送信を諦めた(サーバーに拒否された)時: create は行ごと消し、update/delete は pendingOp を戻す。
    public func revertPending(appId: String, entity: String, recordId: String) {
        var t = table(appId, entity)
        guard let i = t.firstIndex(where: { $0.recordId == recordId }) else { return }
        if t[i].pendingOp == .create { t.remove(at: i) } else { t[i].pendingOp = .none }
        save(appId, entity, t)
    }

    /// テスト/リセット用。appId が変わっても旧 appId の行は消さない(サーバーにも残っているため)ので通常は呼ばない。
    public func removeAll(appId: String) {
        for key in tables.keys where key.hasPrefix(appId + "/") { tables[key] = nil }
        try? FileManager.default.removeItem(at: directory.appendingPathComponent(Self.safe(appId), isDirectory: true))
    }

    /// アカウント削除用(#119): 全 appId の行とファイルを消す。
    public func removeEverything() {
        tables = [:]
        try? FileManager.default.removeItem(at: directory)
    }

    // MARK: - Files

    private func table(_ appId: String, _ entity: String) -> [LocalRecord] {
        let key = "\(appId)/\(entity)"
        if let cached = tables[key] { return cached }
        let loaded = (try? Data(contentsOf: url(appId, entity))).flatMap { try? JSONDecoder().decode([LocalRecord].self, from: $0) } ?? []
        tables[key] = loaded
        return loaded
    }

    private func save(_ appId: String, _ entity: String, _ rows: [LocalRecord]) {
        tables["\(appId)/\(entity)"] = rows
        do {
            let dir = directory.appendingPathComponent(Self.safe(appId), isDirectory: true)
            try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
            try JSONEncoder().encode(rows).write(to: url(appId, entity), options: .atomic)
        } catch {
            print("[Metamon] RecordStore save failed: \(error)")
        }
    }

    private func url(_ appId: String, _ entity: String) -> URL {
        directory.appendingPathComponent(Self.safe(appId), isDirectory: true).appendingPathComponent("\(Self.safe(entity)).json")
    }

    static func safe(_ s: String) -> String {
        String(s.map { $0.isLetter || $0.isNumber || $0 == "-" || $0 == "_" ? $0 : "_" })
    }
}
