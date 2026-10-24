import Foundation
import MetamonDSL

/// インメモリの appconfig。サーバー無しで DSL レンダラーを動かすためのもの。
public actor InMemoryAppConfigClient: AppConfigClient {
    private var tables: [String: [Record]]
    private let document: @Sendable () throws -> AppDocument

    /// - Parameters:
    ///   - seed: entity id → 初期レコード
    ///   - document: `fetchDocument` が返す DSL(既定はバンドルの `initial`)
    public init(seed: [String: [Record]] = [:], document: @escaping @Sendable () throws -> AppDocument = { try DSLExamples.document(DSLExamples.initial) }) {
        tables = seed
        self.document = document
    }

    public func fetchDocument(appId: String) async throws -> AppDocument {
        try document()
    }

    public func list(appId: String, entity: String) async throws -> [Record] {
        tables[entity] ?? []
    }

    public func create(appId: String, entity: String, data: [String: JSONValue]) async throws -> Record {
        let now = ISO8601DateFormatter().string(from: .now)
        let record = Record(id: UUID().uuidString, data: data, createdAt: now, updatedAt: now)
        tables[entity, default: []].append(record)
        return record
    }

    public func update(appId: String, entity: String, id: String, data: [String: JSONValue]) async throws -> Record {
        guard let i = tables[entity]?.firstIndex(where: { $0.id == id }) else {
            throw APIError.http(status: 404, body: "\(entity)/\(id) not found")
        }
        var r = tables[entity]![i]
        for (k, v) in data { r.data[k] = v }
        r.updatedAt = ISO8601DateFormatter().string(from: .now)
        tables[entity]![i] = r
        return r
    }

    public func delete(appId: String, entity: String, id: String) async throws {
        tables[entity]?.removeAll { $0.id == id }
    }
}

public enum MockSeed {
    /// デモ3本(docs/dsl/examples)の entity id に合わせた seed データ。
    public static func demoRecords(now: Date = .now) -> [String: [Record]] {
        let today = FieldCoding.dateFormatter.string(from: now)
        let iso = ISO8601DateFormatter().string(from: now)
        return [
            "expense": [
                Record(id: "1", data: ["amount": 980, "category": "食費", "memo": "ランチ", "date": .string(today)], createdAt: iso),
                Record(id: "2", data: ["amount": 420, "category": "交通", "memo": "電車", "date": .string(today)], createdAt: iso),
            ],
            "workout": [
                Record(id: "1", data: ["exercise": "スクワット", "reps": 20, "weight": 40, "done": true, "date": .string(iso)], createdAt: iso),
            ],
            "memo": [
                Record(id: "1", data: ["note": "きれいな紅葉", "date": .string(iso)], createdAt: iso),
            ],
        ]
    }
}
