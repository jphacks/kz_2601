import Foundation
import MetamonDSL

/// appconfig サービス: DSL 取得 + entity レコード API。
/// 生成アプリのユーザーデータはすべてここ(Firestore)に保存する。iOS ローカルはキャッシュのみ。
public protocol AppConfigClient: Sendable {
    func fetchDocument(appId: String) async throws -> AppDocument
    func list(appId: String, entity: String) async throws -> [Record]
    func create(appId: String, entity: String, data: [String: JSONValue]) async throws -> Record
    func update(appId: String, entity: String, id: String, data: [String: JSONValue]) async throws -> Record
    func delete(appId: String, entity: String, id: String) async throws
}
