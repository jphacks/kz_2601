import Foundation
import MetamonDSL

/// appconfig の REST 実装(docs/backend/openapi-appconfig.yaml / backend/internal/appconfig)。
/// - 未知の appId は GET /dsl 時にサーバーが version 1(初期状態)で自動作成する(iOS は `POST /apps` を呼ばない)
/// - `GET /dsl` は `ETag: "<version>"` を返す。ポーリングは `If-None-Match` を付けて 304 なら何もしない
/// - records: `pageSize` ≤ 200・`nextPageToken` で続きを取る。`POST` は `clientId`(UUID)で冪等(通信断の再送で二重登録しない)
/// - 取得した DSL 封筒は `DocumentCache` に保存し、オフライン起動時の復元に使う
public struct HTTPAppConfigClient: AppConfigClient {
    public let baseURL: URL
    public let token: any TokenProvider
    public var cache: DocumentCache?

    public init(baseURL: URL, token: any TokenProvider, cache: DocumentCache? = .shared) {
        self.baseURL = baseURL
        self.token = token
        self.cache = cache
    }

    static let pageSize = 200
    /// 1 entity あたり最大 1000 件まで(それ以上は画面でも持て余す)
    static let maxPages = 5

    private struct ListResponse: Decodable {
        var items: [Record]
        var nextPageToken: String?
    }
    private struct VersionsResponse: Decodable { var items: [VersionSummary] }
    private struct AppsResponse: Decodable {
        var items: [AppSummary]
        var nextPageToken: String?
    }
    private struct NameBody: Encodable { var name: String }
    private struct OrderBody: Encodable { var ids: [String] }
    private struct RecordBody: Encodable { var data: [String: JSONValue] }
    private struct CreateBody: Encodable {
        var data: [String: JSONValue]
        var clientId: String
        /// nil はキーごと送らない(backend が「匿名」を付ける)
        var authorName: String?
    }

    // MARK: - Apps

    /// `GET /v1/apps`(最大 5 ページ)。成功したら `AppListCache` に保存し、オフライン時の変身一覧に使う。
    public func listApps() async throws -> [AppSummary] {
        var items: [AppSummary] = []
        var token: String?
        for _ in 0..<Self.maxPages {
            var path = "v1/apps?pageSize=\(Self.pageSize)"
            if let token { path += "&pageToken=\(token)" }
            let page = try JSONDecoder().decode(AppsResponse.self, from: try await request("GET", path))
            items += page.items
            guard let next = page.nextPageToken, !next.isEmpty else { break }
            token = next
        }
        AppListCache.shared.store(items)
        return items
    }

    /// `DELETE /v1/apps/{appId}`(物理削除・#180)。成功したらオフライン用の一覧キャッシュからも外す。
    public func deleteApp(appId: String) async throws {
        _ = try await request("DELETE", "v1/apps/\(appId)")
        AppListCache.shared.store(AppListCache.shared.load().filter { $0.id != appId })
    }

    /// `PATCH /v1/apps/{appId}`(名前・#277)。オフライン用の一覧キャッシュも書き換える。
    public func renameApp(appId: String, name: String) async throws -> AppSummary {
        let app = try JSONDecoder().decode(AppSummary.self, from: try await request("PATCH", "v1/apps/\(appId)", body: NameBody(name: name)))
        AppListCache.shared.store(AppListCache.shared.load().map { $0.id == appId ? app : $0 })
        return app
    }

    /// `POST /v1/apps/{appId}/duplicate`(#277)。応答は AppWithDSL だが一覧に要る AppSummary だけ読む。
    public func duplicateApp(appId: String, name: String) async throws -> AppSummary {
        let app = try JSONDecoder().decode(AppSummary.self, from: try await request("POST", "v1/apps/\(appId)/duplicate", body: NameBody(name: name)))
        AppListCache.shared.store(AppListCache.shared.load() + [app])
        return app
    }

    /// `PUT /v1/apps/order`(#277)。
    public func reorderApps(ids: [String]) async throws {
        _ = try await request("PUT", "v1/apps/order", body: OrderBody(ids: ids))
        AppListCache.shared.store(AppListEdit.applyingOrder(ids, to: AppListCache.shared.load()))
    }

    // MARK: - DSL

    public func fetchDocument(appId: String) async throws -> AppDocument {
        let data = try await request("GET", "v1/apps/\(appId)/dsl")
        return try store(appId: appId, envelope: data)
    }

    public func fetchDocumentIfChanged(appId: String, currentVersion: Int) async throws -> AppDocument? {
        let r = try await HTTP.exchange("GET", baseURL: baseURL, path: "v1/apps/\(appId)/dsl", body: nil, token: token,
                                        headers: ["If-None-Match": Self.etag(currentVersion)])
        if r.status == 304 { return nil }
        let doc = try store(appId: appId, envelope: try HTTP.check(r))
        return doc.version != currentVersion ? doc : nil
    }

    public func listVersions(appId: String) async throws -> [VersionSummary] {
        try JSONDecoder().decode(VersionsResponse.self, from: try await request("GET", "v1/apps/\(appId)/versions?pageSize=50")).items
    }

    public func restore(appId: String, toVersion version: Int) async throws -> AppDocument {
        // 201 の本文は VersionSummary + 復元後の dsl(DSLEnvelope 互換・#67)。そのまま現行 DSL として使う
        let data = try await request("POST", "v1/apps/\(appId)/versions/\(version)/restore")
        return try store(appId: appId, envelope: data)
    }

    /// backend の `etag(v)` と同じ `"7"` 形式。
    static func etag(_ version: Int) -> String { "\"\(version)\"" }

    private func store(appId: String, envelope: Data) throws -> AppDocument {
        let doc = try AppDocument.decode(envelope)
        cache?.store(appId: appId, envelope: envelope)
        return doc
    }

    // MARK: - Records

    public func list(appId: String, entity: String) async throws -> [Record] {
        var items: [Record] = []
        var token: String?
        for _ in 0..<Self.maxPages {
            var path = "v1/apps/\(appId)/entities/\(entity)/records?pageSize=\(Self.pageSize)"
            if let token { path += "&pageToken=\(token)" }
            let page = try JSONDecoder().decode(ListResponse.self, from: try await request("GET", path))
            items += page.items
            guard let next = page.nextPageToken, !next.isEmpty else { break }
            token = next
        }
        return items
    }

    public func create(appId: String, entity: String, data: [String: JSONValue]) async throws -> Record {
        try await create(appId: appId, entity: entity, data: data, clientId: UUID().uuidString.lowercased())
    }

    public func create(appId: String, entity: String, data: [String: JSONValue], clientId: String) async throws -> Record {
        try await create(appId: appId, entity: entity, data: data, clientId: clientId, authorName: nil)
    }

    public func create(appId: String, entity: String, data: [String: JSONValue], clientId: String, authorName: String?) async throws -> Record {
        let body = CreateBody(data: data, clientId: clientId, authorName: authorName)
        let path = "v1/apps/\(appId)/entities/\(entity)/records"
        do {
            return try await decodeRecord(try await request("POST", path, body: body))
        } catch let error as URLError where Self.isRetryable(error) {
            // 送ったかどうか分からない通信断。同じ clientId で再送すればサーバー側で同一レコードに畳まれる
            return try await decodeRecord(try await request("POST", path, body: body))
        }
    }

    public func update(appId: String, entity: String, id: String, data: [String: JSONValue]) async throws -> Record {
        try await decodeRecord(try await request("PATCH", "v1/apps/\(appId)/entities/\(entity)/records/\(id)", body: RecordBody(data: data)))
    }

    public func delete(appId: String, entity: String, id: String) async throws {
        _ = try await request("DELETE", "v1/apps/\(appId)/entities/\(entity)/records/\(id)")
    }

    static func isRetryable(_ error: URLError) -> Bool {
        [.timedOut, .networkConnectionLost, .cannotConnectToHost, .notConnectedToInternet].contains(error.code)
    }

    private func decodeRecord(_ data: Data) throws -> Record {
        try JSONDecoder().decode(Record.self, from: data)
    }

    private func request(_ method: String, _ path: String, body: (any Encodable)? = nil) async throws -> Data {
        try await HTTP.send(method, baseURL: baseURL, path: path, body: body, token: token)
    }
}
