import Foundation
import MetamonDSL

/// `GET /v1/apps/{appId}/versions` の 1 件(openapi `VersionSummary`)。DSL 本文は含まない。
public struct VersionSummary: Decodable, Sendable, Equatable {
    public struct Source: Decodable, Sendable, Equatable {
        public var kind: String?
        public var mode: String?
        public var summary: String?
        public var restoredFrom: Int?
    }

    public var appId: String?
    public var version: Int
    public var parentVersion: Int?
    public var source: Source?
    public var createdAt: String?

    public init(appId: String? = nil, version: Int, parentVersion: Int? = nil, source: Source? = nil, createdAt: String? = nil) {
        self.appId = appId
        self.version = version
        self.parentVersion = parentVersion
        self.source = source
        self.createdAt = createdAt
    }
}

/// `GET /v1/apps` の 1 件(openapi `AppSummary`)。uid スコープの「自分のアプリ一覧」。name は ai が DSL を書くたびに `dsl.app.name` で更新される。
public struct AppSummary: Codable, Sendable, Equatable, Identifiable, Hashable {
    public var id: String
    public var name: String
    public var currentVersion: Int
    /// `active` / `archived`
    public var status: String
    public var createdAt: String?
    public var updatedAt: String?
    /// 変身一覧で自分が並べた位置(1 が先頭・#277)。並べ替えたことが無ければ nil。
    public var sortIndex: Int?

    public init(id: String, name: String, currentVersion: Int, status: String = "active", createdAt: String? = nil, updatedAt: String? = nil, sortIndex: Int? = nil) {
        self.id = id
        self.name = name
        self.currentVersion = currentVersion
        self.status = status
        self.createdAt = createdAt
        self.updatedAt = updatedAt
        self.sortIndex = sortIndex
    }

    public var isArchived: Bool { status == "archived" }
    /// `GET /dsl` の自動作成(version 1 = 初期状態)のまま一度も生成されていないアプリ。一覧には出さない。
    public var isBlank: Bool { currentVersion <= 1 }
    public var updatedDate: Date? { updatedAt.flatMap(Self.parseDate) }

    private static let isoFractional: ISO8601DateFormatter = {
        let f = ISO8601DateFormatter()
        f.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return f
    }()
    private static let iso = ISO8601DateFormatter()
    static func parseDate(_ s: String) -> Date? { isoFractional.date(from: s) ?? iso.date(from: s) }

    /// 変身一覧に出す並び: archived と未生成(blank)を除く(現在のアプリは常に含める)。
    /// - 自分で並べたことがある(どれかに `sortIndex` がある・#277): 並べた後に増えたアプリ(`sortIndex` なし)を新しい順で先頭、続けて並べた順
    /// - 無い: 現在のアプリを先頭、残りは更新日時の新しい順
    public static func visible(_ apps: [AppSummary], current: String?) -> [AppSummary] {
        var seen = Set<String>()
        let kept = apps.filter { app in
            guard seen.insert(app.id).inserted, !app.isArchived else { return false }
            return app.id == current || !app.isBlank
        }
        let newer: (AppSummary, AppSummary) -> Bool = { ($0.updatedDate ?? .distantPast) > ($1.updatedDate ?? .distantPast) }
        if kept.contains(where: { $0.sortIndex != nil }) {
            return kept.sorted { a, b in
                switch (a.sortIndex, b.sortIndex) {
                case let (x?, y?): return x != y ? x < y : newer(a, b)
                case (nil, _?): return true
                case (_?, nil): return false
                case (nil, nil): return newer(a, b)
                }
            }
        }
        return kept.sorted { a, b in
            if (a.id == current) != (b.id == current) { return a.id == current }
            return newer(a, b)
        }
    }
}

/// appconfig サービス: DSL 取得 + version 履歴 + entity レコード API(docs/backend/openapi-appconfig.yaml)。
/// 生成アプリのユーザーデータはすべてここ(Firestore)に保存する。iOS ローカルはキャッシュのみ。
/// DSL の書き込み(PUT/PATCH /dsl)は ai 経由なので iOS からは行わない。iOS が version を進めるのは restore だけ。
public protocol AppConfigClient: Sendable {
    /// `GET /v1/apps`(uid スコープ)。変身一覧(複数アプリ切替)が使う。実装が無ければ空。
    func listApps() async throws -> [AppSummary]
    /// `DELETE /v1/apps/{appId}`(#180)。サーバーは物理削除し、自分のシェアリンク・「探す」掲載・共有データも消す。
    func deleteApp(appId: String) async throws
    /// `PATCH /v1/apps/{appId}` の `name`(#277)。サーバーは `dsl.app.name` にも書き、新しい version として積む。
    func renameApp(appId: String, name: String) async throws -> AppSummary
    /// `POST /v1/apps/{appId}/duplicate`(#277)。画面の定義だけを新しい appId に複製する(記録は複製しない)。
    func duplicateApp(appId: String, name: String) async throws -> AppSummary
    /// `PUT /v1/apps/order`(#277)。並べた順(先頭から)を保存する。
    func reorderApps(ids: [String]) async throws
    func fetchDocument(appId: String) async throws -> AppDocument
    /// ETag(`If-None-Match: "<version>"`)付き取得。サーバーが同じ version なら nil(304)。ホットリロードのポーリングが使う。
    func fetchDocumentIfChanged(appId: String, currentVersion: Int) async throws -> AppDocument?
    /// 新しい順。
    func listVersions(appId: String) async throws -> [VersionSummary]
    /// `POST /versions/{version}/restore` → 新 version として積み直した最新 DSL を返す。
    func restore(appId: String, toVersion version: Int) async throws -> AppDocument

    func list(appId: String, entity: String) async throws -> [Record]
    func create(appId: String, entity: String, data: [String: JSONValue]) async throws -> Record
    /// 冪等キー付き作成。`SyncQueue` が再送のたびに同じ `clientId` を渡す(サーバーは同一レコードを返す)。
    func create(appId: String, entity: String, data: [String: JSONValue], clientId: String) async throws -> Record
    /// 投稿者の表示名付き(shared / public の entity・#250)。backend は private の entity では無視する。
    func create(appId: String, entity: String, data: [String: JSONValue], clientId: String, authorName: String?) async throws -> Record
    func update(appId: String, entity: String, id: String, data: [String: JSONValue]) async throws -> Record
    func delete(appId: String, entity: String, id: String) async throws
    /// 自分の印を付ける(on)/ 外す(#255・`PUT|DELETE .../records/{id}/marks/{mark}`・冪等)。サーバーの件数と自分の状態を返す。
    func setMark(appId: String, entity: String, id: String, mark: String, on: Bool) async throws -> MarkState
}

public extension AppConfigClient {
    func listApps() async throws -> [AppSummary] { [] }

    /// 印を扱わない実装(古いモック等)。
    func setMark(appId: String, entity: String, id: String, mark: String, on: Bool) async throws -> MarkState {
        throw APIError.server(status: 501, code: "unsupported", message: "marks are not supported by this client")
    }
    func deleteApp(appId: String) async throws {}
    func renameApp(appId: String, name: String) async throws -> AppSummary { throw AppListEdit.unsupported }
    func duplicateApp(appId: String, name: String) async throws -> AppSummary { throw AppListEdit.unsupported }
    func reorderApps(ids: [String]) async throws { throw AppListEdit.unsupported }

    /// 冪等キーを使わない実装(インメモリ等)向けの既定。
    func create(appId: String, entity: String, data: [String: JSONValue], clientId: String) async throws -> Record {
        try await create(appId: appId, entity: entity, data: data)
    }

    /// 表示名を扱わない実装(インメモリ等)向けの既定。
    func create(appId: String, entity: String, data: [String: JSONValue], clientId: String, authorName: String?) async throws -> Record {
        try await create(appId: appId, entity: entity, data: data, clientId: clientId)
    }

    func fetchDocumentIfChanged(appId: String, currentVersion: Int) async throws -> AppDocument? {
        let doc = try await fetchDocument(appId: appId)
        return doc.version != currentVersion ? doc : nil
    }
}
