import XCTest
import MetamonDSL
@testable import MetamonRenderer

/// aiButton の entity 入力: クラウドの AI へは共有 entity の自分の記録だけ・端末内 AI は絞らない・件数の上限。
final class AIInputRecordsTests: XCTestCase {
    private func entity(scope: String?) throws -> Entity {
        let scopeJSON = scope.map { #","scope":"\#($0)""# } ?? ""
        return try JSONDecoder().decode(Entity.self, from: Data(#"""
            {"id":"posts","name":"投稿","fields":[{"id":"body","name":"本文","type":"text"}]\#(scopeJSON)}
            """#.utf8))
    }

    private func component(_ json: String) throws -> Component {
        try JSONDecoder().decode(Component.self, from: Data(json.utf8))
    }

    private let mixed = [
        Record(id: "a", data: ["body": "自分の投稿"], updatedAt: "2026-11-01T00:00:00Z", author: "me"),
        Record(id: "b", data: ["body": "他の人の投稿"], updatedAt: "2026-11-02T00:00:00Z", author: "other"),
        Record(id: "c", data: ["body": "未送信の自分の分"], updatedAt: "2026-11-03T00:00:00Z"),
    ]

    func testOnlineSharedKeepsOnlyOwnRecords() throws {
        for scope in ["shared", "public"] {
            let out = try AIInputRecords.select(mixed, in: try entity(scope: scope), route: .online, uid: "me")
            XCTAssertEqual(Set(out.map(\.id)), ["a", "c"], "\(scope): 他の人の記録は送らない・author 無しは自分の分")
        }
    }

    func testOnDeviceAndPrivateAreNotFiltered() throws {
        let shared = try entity(scope: "shared")
        XCTAssertEqual(try AIInputRecords.select(mixed, in: shared, route: .onDevice, uid: "me").map(\.id), ["a", "b", "c"],
                       "端末内 AI はデータが端末を出ないので絞らない")
        let priv = try entity(scope: nil)
        XCTAssertEqual(Set(try AIInputRecords.select(mixed, in: priv, route: .online, uid: "me").map(\.id)), ["a", "b", "c"])
    }

    func testUnknownUidSendsOnlyRecordsWithoutAuthor() throws {
        let out = try AIInputRecords.select(mixed, in: try entity(scope: "shared"), route: .online, uid: nil)
        XCTAssertEqual(out.map(\.id), ["c"])
    }

    func testNoOwnRecordsLeftThrowsCalmMessage() throws {
        let others = [Record(id: "b", data: ["body": "他の人の投稿"], author: "other")]
        XCTAssertThrowsError(try AIInputRecords.select(others, in: try entity(scope: "shared"), route: .online, uid: "me")) {
            XCTAssertEqual($0 as? AIInputRecords.SelectError, .noOwnRecords)
            XCTAssertEqual($0.localizedDescription, "AI に渡せるのは自分が書いた記録だけです")
        }
        // 元から 0 件なら絞った結果ではないので投げない(各ビルダーの「先に記録を追加してください」に任せる)
        XCTAssertEqual(try AIInputRecords.select([], in: try entity(scope: "shared"), route: .online, uid: "me"), [])
    }

    func testOnlineKeepsNewestMaxRecords() throws {
        let many = (0..<30).map { i in
            Record(id: "r\(i)", data: ["body": .string("\(i)")], updatedAt: String(format: "2026-10-%02dT00:00:00Z", i + 1))
        }
        let out = try AIInputRecords.select(many, in: try entity(scope: nil), route: .online, uid: "me")
        XCTAssertEqual(out.count, AITextLimits.maxRecords)
        XCTAssertEqual(out.first?.id, "r29", "新しい順")
        XCTAssertEqual(try AIInputRecords.select(many, in: try entity(scope: nil), route: .onDevice, uid: "me").count, 30)
    }

    func testInputTextLimitIsCapped() throws {
        let e = try entity(scope: nil)
        let many = (0..<80).map { i in
            Record(id: "r\(i)", data: ["body": .string("\(i)")], updatedAt: String(format: "2026-%02d-01T00:00:00Z", i % 12 + 1))
        }
        let big = try component(#"{"type":"aiButton","task":"summarize","entity":"posts","limit":999,"output":"x"}"#)
        let text = AIRequestBuilder.inputText(big, entity: e, values: [:], sourceEntity: e, records: many)
        XCTAssertEqual(text.split(separator: "\n").count, AIRequestBuilder.maxLimit, "limit は DSL の上限で頭打ち")
        let small = try component(#"{"type":"aiButton","task":"summarize","entity":"posts","limit":5,"output":"x"}"#)
        XCTAssertEqual(AIRequestBuilder.inputText(small, entity: e, values: [:], sourceEntity: e, records: many)
            .split(separator: "\n").count, 5)
    }
}
