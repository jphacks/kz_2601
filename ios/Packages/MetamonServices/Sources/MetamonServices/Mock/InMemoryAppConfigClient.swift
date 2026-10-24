import Foundation
import MetamonDSL

/// インメモリの appconfig。サーバー無しで DSL レンダラーを動かすためのもの。
public actor InMemoryAppConfigClient: AppConfigClient {
    private var tables: [String: [Record]]
    private let document: @Sendable () throws -> AppDocument
    /// `listApps` が返す一覧(モックの変身一覧用。既定は空。`setApps` で差し替え)。
    private var apps: [AppSummary] = []

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

    public func listApps() async throws -> [AppSummary] { apps }

    public func setApps(_ apps: [AppSummary]) { self.apps = apps }

    /// 履歴は持たない(モックの undo は `AppRuntime` のローカル履歴にフォールバックする)。
    public func listVersions(appId: String) async throws -> [VersionSummary] { [] }

    public func restore(appId: String, toVersion version: Int) async throws -> AppDocument {
        throw APIError.server(status: 404, code: "not_found", message: "version not found (in-memory)")
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
            // warikan(#101): 3 人・3 件。合計 ¥15,000 → 1 人 ¥5,000、精算は じろう→たろう ¥5,000 / はなこ→たろう ¥500
            "members": [
                Record(id: "m1", data: ["name": "たろう"], createdAt: iso),
                Record(id: "m2", data: ["name": "はなこ"], createdAt: iso),
                Record(id: "m3", data: ["name": "じろう"], createdAt: iso),
            ],
            "expenses": [
                Record(id: "e1", data: ["title": "宿", "amount": 9000, "payer": "m1", "date": .string(today)], createdAt: iso),
                Record(id: "e2", data: ["title": "夕食", "amount": 4500, "payer": "m2", "date": .string(today)], createdAt: iso),
                Record(id: "e3", data: ["title": "ガソリン", "amount": 1500, "payer": "m1", "date": .string(today)], createdAt: iso),
            ],
        ]
    }
}
