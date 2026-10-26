import Foundation
import MetamonDSL

/// 機内モード擬似: `until` までレコード API を `URLError(.notConnectedToInternet)` で落とす(DSL 取得は通す)。
/// シミュレータ E2E(`METAMON_E2E_OFFLINE_SECONDS`)とテストで、オフライン保存 → 復帰で送信される経路を通すために使う。
public final class OfflineSimulatingAppConfigClient: AppConfigClient, @unchecked Sendable {
    public let inner: any AppConfigClient
    private let lock = NSLock()
    private var offlineUntil: Date?
    /// 通した / 落とした呼び出しの記録(テスト用)
    public private(set) var log: [String] = []

    public init(inner: any AppConfigClient, offlineUntil: Date? = nil) {
        self.inner = inner
        self.offlineUntil = offlineUntil
    }

    public var isOffline: Bool { lock.withLock { offlineUntil.map { $0 > .now } ?? false } }

    public func setOffline(_ offline: Bool) {
        lock.withLock { offlineUntil = offline ? .distantFuture : nil }
    }

    private func gate(_ name: String) throws {
        let offline = isOffline
        lock.withLock { log.append((offline ? "drop " : "pass ") + name) }
        if offline { throw URLError(.notConnectedToInternet) }
    }

    public func fetchDocument(appId: String) async throws -> AppDocument { try await inner.fetchDocument(appId: appId) }
    public func fetchDocumentIfChanged(appId: String, currentVersion: Int) async throws -> AppDocument? {
        try await inner.fetchDocumentIfChanged(appId: appId, currentVersion: currentVersion)
    }
    public func listVersions(appId: String) async throws -> [VersionSummary] { try await inner.listVersions(appId: appId) }
    public func restore(appId: String, toVersion version: Int) async throws -> AppDocument { try await inner.restore(appId: appId, toVersion: version) }

    public func list(appId: String, entity: String) async throws -> [Record] {
        try gate("list \(entity)")
        return try await inner.list(appId: appId, entity: entity)
    }

    public func create(appId: String, entity: String, data: [String: JSONValue]) async throws -> Record {
        try await create(appId: appId, entity: entity, data: data, clientId: UUID().uuidString.lowercased())
    }

    public func create(appId: String, entity: String, data: [String: JSONValue], clientId: String) async throws -> Record {
        try await create(appId: appId, entity: entity, data: data, clientId: clientId, authorName: nil)
    }

    public func create(appId: String, entity: String, data: [String: JSONValue], clientId: String, authorName: String?) async throws -> Record {
        try gate("create \(entity) \(clientId)")
        return try await inner.create(appId: appId, entity: entity, data: data, clientId: clientId, authorName: authorName)
    }

    public func update(appId: String, entity: String, id: String, data: [String: JSONValue]) async throws -> Record {
        try gate("update \(entity)/\(id)")
        return try await inner.update(appId: appId, entity: entity, id: id, data: data)
    }

    public func setMark(appId: String, entity: String, id: String, mark: String, on: Bool) async throws -> MarkState {
        try gate("mark \(entity)/\(id) \(mark) \(on)")
        return try await inner.setMark(appId: appId, entity: entity, id: id, mark: mark, on: on)
    }

    public func delete(appId: String, entity: String, id: String) async throws {
        try gate("delete \(entity)/\(id)")
        try await inner.delete(appId: appId, entity: entity, id: id)
    }
}
