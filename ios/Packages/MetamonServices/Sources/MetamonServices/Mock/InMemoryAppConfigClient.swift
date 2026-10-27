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

    public func deleteApp(appId: String) async throws {
        apps.removeAll { $0.id == appId }
    }

    public func renameApp(appId: String, name: String) async throws -> AppSummary {
        guard let i = apps.firstIndex(where: { $0.id == appId }) else {
            throw APIError.server(status: 404, code: "not_found", message: "app not found (in-memory)")
        }
        apps[i].name = name
        apps[i].currentVersion += 1
        return apps[i]
    }

    public func duplicateApp(appId: String, name: String) async throws -> AppSummary {
        guard apps.contains(where: { $0.id == appId }) else {
            throw APIError.server(status: 404, code: "not_found", message: "app not found (in-memory)")
        }
        let now = ISO8601DateFormatter().string(from: .now)
        let copy = AppSummary(id: UUID().uuidString.lowercased(), name: name, currentVersion: 2, createdAt: now, updatedAt: now)
        apps.append(copy)
        return copy
    }

    public func reorderApps(ids: [String]) async throws {
        apps = AppListEdit.applyingOrder(ids, to: apps)
    }

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

    /// 表示名付きの作成(#250)。appconfig と同じく作者を付ける(scope は知らないので、表示名が来た時だけ。作者はモックの uid)。
    public func create(appId: String, entity: String, data: [String: JSONValue], clientId: String, authorName: String?) async throws -> Record {
        var record = try await create(appId: appId, entity: entity, data: data)
        guard let authorName, let i = tables[entity]?.lastIndex(where: { $0.id == record.id }) else { return record }
        record.author = MockAuthService.fixedUID
        record.authorName = authorName
        tables[entity]![i] = record
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

    /// 印(#255)。モックは利用者が 1 人なので、レコードの marks をそのまま 1 人分だけ増減する。
    public func setMark(appId: String, entity: String, id: String, mark: String, on: Bool) async throws -> MarkState {
        guard let i = tables[entity]?.firstIndex(where: { $0.id == id }) else {
            throw APIError.server(status: 404, code: "not_found", message: "record not found")
        }
        var r = tables[entity]![i]
        var st = r.markState(mark)
        if st.mine != on { st = st.toggled() }
        r.marks = (r.marks ?? [:]).merging([mark: st]) { _, new in new }
        tables[entity]![i] = r
        return st
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
            // chat(#260): 相手 2 人 + 自分(モックの uid)の会話。古い順に下へ積む
            "messages": chatSeed(now: now),
            // shopping(#108): 未購入 3 件 + 購入済み 1 件(一覧の丸でチェック)
            "groceries": [
                Record(id: "g1", data: ["name": "牛乳", "category": "食品", "qty": 1, "done": false, "added": .string(today)], createdAt: iso),
                Record(id: "g2", data: ["name": "卵", "category": "食品", "qty": 1, "done": false, "added": .string(today)], createdAt: iso),
                Record(id: "g3", data: ["name": "トイレットペーパー", "category": "日用品", "qty": 2, "done": false, "added": .string(today)], createdAt: iso),
                Record(id: "g4", data: ["name": "パン", "category": "食品", "qty": 1, "done": true, "added": .string(today)], createdAt: iso),
            ],
            // medicine(#132/#172): 今日まで 3 日連続の 1 件・昨日飲んだ 1 件(今日は未チェック・履歴なしの旧形式)・未服用 1 件
            "medicine": [
                Record(id: "d1", data: ["name": "血圧の薬", "timing": "朝", "taken_on": .string(today),
                                        DailyCheck.historyKey("taken_on"): .array((0..<3).reversed().map { .string(FieldCoding.dateFormatter.string(from: now.addingTimeInterval(-86_400 * Double($0)))) })], createdAt: iso),
                Record(id: "d2", data: ["name": "ビタミン剤", "timing": "昼", "taken_on": .string(FieldCoding.dateFormatter.string(from: now.addingTimeInterval(-86_400)))], createdAt: iso),
                Record(id: "d3", data: ["name": "胃薬", "timing": "夜"], createdAt: iso),
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
            // dinner(#263): randomPicker の候補 4 件
            "dish": [
                Record(id: "d1", data: ["name": "カレー", "genre": "洋食", "memo": "辛口"], createdAt: iso),
                Record(id: "d2", data: ["name": "肉じゃが", "genre": "和食"], createdAt: iso),
                Record(id: "d3", data: ["name": "麻婆豆腐", "genre": "中華"], createdAt: iso),
                Record(id: "d4", data: ["name": "焼き魚", "genre": "和食", "memo": "鮭"], createdAt: iso),
            ],
            // timetable(#234): 月1・水2(同じマスに 2 件 → 「+1」)・金3
            "lecture": [
                Record(id: "l1", data: ["subject": "線形代数", "weekday": "月", "period": 1, "room": "A101"], createdAt: iso),
                Record(id: "l2", data: ["subject": "英語", "weekday": "水", "period": 2, "room": "B203"], createdAt: iso),
                Record(id: "l3", data: ["subject": "英語演習", "weekday": "水", "period": 2, "room": "B204"], createdAt: iso),
                Record(id: "l4", data: ["subject": "プログラミング", "weekday": "金", "period": 3, "room": "PC室"], createdAt: iso),
            ],
            // recipes(#255): ほかの人のレシピ 3 件。保存の件数はほかの人の分、カレーだけ自分も保存済み
            "recipes": [
                Record(id: "r1", data: ["title": "肉じゃが", "minutes": 30], createdAt: iso, author: "u-hanako", authorName: "はなこ",
                       marks: ["save": MarkState(count: 3, mine: false)]),
                Record(id: "r2", data: ["title": "チキンカレー", "minutes": 45], createdAt: iso, author: "u-jiro", authorName: "じろう",
                       marks: ["save": MarkState(count: 2, mine: true)]),
                Record(id: "r3", data: ["title": "だし巻き卵", "minutes": 10], createdAt: iso, author: "u-hanako", authorName: "はなこ",
                       marks: ["save": MarkState(count: 0, mine: false)]),
            ],
            // post_comments(#256): 投稿 2 件(投稿のフォーム →「コメントを書く」で親が入る)
            "posts": [
                Record(id: "post1", data: ["body": "金沢でハッカソンに参加中", "posted": .string(iso)], createdAt: iso),
                Record(id: "post2", data: ["body": "新しいカフェを見つけた", "posted": .string(iso)], createdAt: iso),
            ],
            // bmi(#101): 65kg/170cm → 22.5、70kg/175cm → 22.9
            "body": [
                Record(id: "b1", data: ["date": .string(today), "weight": 65, "height": 170], createdAt: iso),
                Record(id: "b2", data: ["date": .string(today), "weight": 70, "height": 175], createdAt: iso),
            ],
            // ranking(#261): 麺屋こがね 3 件(平均 4.3)・らーめん大将 2 件(平均 3.5)・中華そば 一番 1 件(5)→ 店ごとに 1 行
            "review": [
                Record(id: "r1", data: ["shop": "麺屋こがね", "score": 5, "comment": "スープが濃厚", "date": .string(today)], createdAt: iso),
                Record(id: "r2", data: ["shop": "らーめん大将", "score": 3, "comment": "麺が少し柔らかめ", "date": .string(today)], createdAt: iso),
                Record(id: "r3", data: ["shop": "麺屋こがね", "score": 4, "comment": "チャーシューが絶品", "date": .string(today)], createdAt: iso),
                Record(id: "r4", data: ["shop": "中華そば 一番", "score": 5, "comment": "あっさり系の名店", "date": .string(today)], createdAt: iso),
                Record(id: "r5", data: ["shop": "麺屋こがね", "score": 4, "comment": "行列は 20 分", "date": .string(today)], createdAt: iso),
                Record(id: "r6", data: ["shop": "らーめん大将", "score": 4, "comment": "替え玉無料", "date": .string(today)], createdAt: iso),
            ],
        ]
    }
}
