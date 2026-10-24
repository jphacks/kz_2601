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

/// appconfig サービス: DSL 取得 + version 履歴 + entity レコード API(docs/backend/openapi-appconfig.yaml)。
/// 生成アプリのユーザーデータはすべてここ(Firestore)に保存する。iOS ローカルはキャッシュのみ。
/// DSL の書き込み(PUT/PATCH /dsl)は ai 経由なので iOS からは行わない。iOS が version を進めるのは restore だけ。
public protocol AppConfigClient: Sendable {
    func fetchDocument(appId: String) async throws -> AppDocument
    /// ETag(`If-None-Match: "<version>"`)付き取得。サーバーが同じ version なら nil(304)。ホットリロードのポーリングが使う。
    func fetchDocumentIfChanged(appId: String, currentVersion: Int) async throws -> AppDocument?
    /// 新しい順。
    func listVersions(appId: String) async throws -> [VersionSummary]
    /// `POST /versions/{version}/restore` → 新 version として積み直した最新 DSL を返す。
    func restore(appId: String, toVersion version: Int) async throws -> AppDocument

    func list(appId: String, entity: String) async throws -> [Record]
    func create(appId: String, entity: String, data: [String: JSONValue]) async throws -> Record
    func update(appId: String, entity: String, id: String, data: [String: JSONValue]) async throws -> Record
    func delete(appId: String, entity: String, id: String) async throws
}

public extension AppConfigClient {
    func fetchDocumentIfChanged(appId: String, currentVersion: Int) async throws -> AppDocument? {
        let doc = try await fetchDocument(appId: appId)
        return doc.version != currentVersion ? doc : nil
    }
}
