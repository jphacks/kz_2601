import Foundation
import MetamonDSL

/// 端末だけに記録を置く appconfig(#1284・Mac)。サーバー(#930)が入るまで、作ったアプリの記録をアプリを終えても残す。
/// 「サーバーの行」は `RecordStore`(appId/entity ごとの JSON)にそのまま置く。印は利用者 1 人分。DSL は持たない(殻が持つ)。
public actor LocalAppConfigClient: AppConfigClient {
    private let rows: RecordStore

    /// - Parameter directory: 行を置く場所。動画・録音は `RecordSyncEngine` 側のフォルダに置くので、ここでは持たない
    public init(directory: URL) {
        rows = RecordStore(directory: directory,
                           videos: LocalVideoStore(directory: directory.appendingPathComponent("videos", isDirectory: true)),
                           audios: LocalAudioStore(directory: directory.appendingPathComponent("audio", isDirectory: true)))
    }

    public func fetchDocument(appId: String) async throws -> AppDocument {
        throw APIError.server(status: 404, code: "not_found", message: "documents are kept by the app (local)")
    }

    public func listVersions(appId: String) async throws -> [VersionSummary] { [] }

    public func restore(appId: String, toVersion version: Int) async throws -> AppDocument {
        throw APIError.server(status: 404, code: "not_found", message: "version not found (local)")
    }

    public func list(appId: String, entity: String) async throws -> [Record] {
        await rows.records(appId: appId, entity: entity)
    }

    /// サーバーと同じ精度(RFC 3339 の小数秒)。同じ秒に作った記録も作った順に並ぶ
    private static func stamp() -> String {
        let f = ISO8601DateFormatter()
        f.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return f.string(from: .now)
    }

    public func create(appId: String, entity: String, data: [String: JSONValue]) async throws -> Record {
        try await create(appId: appId, entity: entity, data: data, clientId: UUID().uuidString.lowercased())
    }

    /// 冪等: 同じ `clientId` の再送は同じ行を返す(appconfig と同じ)
    public func create(appId: String, entity: String, data: [String: JSONValue], clientId: String) async throws -> Record {
        if let sent = await rows.rows(appId: appId, entity: entity).first(where: { $0.clientId == clientId }) { return sent.record }
        let now = Self.stamp()
        let row = LocalRecord(appId: appId, entity: entity, recordId: UUID().uuidString.lowercased(), data: data,
                              createdAt: now, updatedAt: now, clientId: clientId)
        await rows.upsert(row)
        return row.record
    }

    public func update(appId: String, entity: String, id: String, data: [String: JSONValue]) async throws -> Record {
        guard var row = await rows.row(appId: appId, entity: entity, recordId: id) else {
            throw APIError.http(status: 404, body: "\(entity)/\(id) not found")
        }
        row.data.merge(data) { _, new in new }
        row.updatedAt = Self.stamp()
        await rows.upsert(row)
        return row.record
    }

    public func delete(appId: String, entity: String, id: String) async throws {
        await rows.remove(appId: appId, entity: entity, recordId: id)
    }

    public func setMark(appId: String, entity: String, id: String, mark: String, on: Bool) async throws -> MarkState {
        guard let row = await rows.row(appId: appId, entity: entity, recordId: id) else {
            throw APIError.server(status: 404, code: "not_found", message: "record not found")
        }
        var state = row.record.markState(mark)
        if state.mine != on { state = state.toggled() }
        await rows.setMark(appId: appId, entity: entity, recordId: id, mark: mark, state: state)
        return state
    }

    /// アプリを消した時(iOS の `RecordSyncEngine.purge` と同じ): その appId の行を消す
    public func removeAll(appId: String) async {
        await rows.removeAll(appId: appId)
    }
}
