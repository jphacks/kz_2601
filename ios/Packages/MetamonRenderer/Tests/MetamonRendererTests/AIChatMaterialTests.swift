import XCTest
import MetamonDSL
@testable import MetamonRenderer

/// aiChat inputEntity の参考の記録: 自分の記録だけ・hidden と読めない型は送らない・新しい順に limit 件・8 KB まで。
final class AIChatMaterialTests: XCTestCase {
    private func entity(scope: String? = nil, name: String = "日記") throws -> Entity {
        let scopeJSON = scope.map { #","scope":"\#($0)""# } ?? ""
        return try JSONDecoder().decode(Entity.self, from: Data(#"""
            {"id":"diary","name":"\#(name)","fields":[
              {"id":"body","name":"本文","type":"text"},
              {"id":"mood","name":"気分","type":"enum","options":["よい","つらい"]},
              {"id":"secret","name":"ひみつ","type":"string","hidden":true},
              {"id":"pic","name":"写真","type":"photo"},
              {"id":"place","name":"場所","type":"location"}]\#(scopeJSON)}
            """#.utf8))
    }

    private func rec(_ id: String, _ body: String, day: Int, author: String? = nil) -> Record {
        Record(id: id, data: ["body": .string(body), "mood": "つらい", "secret": "答えは 42",
                              "pic": "https://example.com/a.jpg", "place": "35.0,136.0"],
               updatedAt: String(format: "2026-11-%02dT00:00:00Z", day), author: author)
    }

    func testSharedSendsOnlyMyRecordsNewestFirstWithoutHiddenOrMedia() throws {
        let rows = [rec("a", "自分の古い日記", day: 1, author: "me"), rec("b", "他の人の日記", day: 3, author: "other"),
                    rec("c", "未送信の自分の分", day: 2), rec("d", "自分の新しい日記", day: 4, author: "me")]
        for scope in ["shared", "public"] {
            let m = try XCTUnwrap(AIChatMaterial.request(entity: try entity(scope: scope), records: rows, uid: "me", limit: nil))
            XCTAssertEqual(m.entity, "diary")
            XCTAssertEqual(m.name, "日記")
            XCTAssertEqual(m.text, """
                - 本文: 自分の新しい日記 / 気分: つらい
                - 本文: 未送信の自分の分 / 気分: つらい
                - 本文: 自分の古い日記 / 気分: つらい
                """, scope)
            for leak in ["他の人", "答えは 42", "example.com", "35.0"] {
                XCTAssertFalse(m.text.contains(leak), "\(scope): \(leak)")
            }
        }
        let anon = try XCTUnwrap(AIChatMaterial.request(entity: try entity(scope: "shared"), records: rows, uid: nil, limit: nil))
        XCTAssertEqual(anon.text, "- 本文: 未送信の自分の分 / 気分: つらい", "uid が分からなければ author の付いた記録は送らない")
    }

    func testNoOwnRecordsMeansNoMaterial() throws {
        XCTAssertNil(AIChatMaterial.request(entity: try entity(scope: "shared"), records: [rec("b", "x", day: 1, author: "other")],
                                            uid: "me", limit: nil))
        XCTAssertNil(AIChatMaterial.request(entity: try entity(), records: [], uid: "me", limit: nil))
    }

    func testLimitIsClampedTo1Through20() throws {
        let many = (1...30).map { rec("r\($0)", "日記\($0)", day: $0 % 28 + 1) }
        let e = try entity()
        XCTAssertEqual(AIChatMaterial.select(many, uid: "me", limit: nil).count, 20)
        XCTAssertEqual(AIChatMaterial.select(many, uid: "me", limit: 99).count, 20)
        XCTAssertEqual(AIChatMaterial.select(many, uid: "me", limit: 3).count, 3)
        XCTAssertEqual(AIChatMaterial.select(many, uid: "me", limit: 0).count, 1)
        let three = try XCTUnwrap(AIChatMaterial.request(entity: e, records: many, uid: "me", limit: 3))
        XCTAssertEqual(three.text.split(separator: "\n").count, 3)
    }

    func testTextStaysWithin8KBDroppingOlderRecords() throws {
        let long = (1...20).map { rec("r\($0)", String(repeating: "あ", count: 1000) + "\($0)", day: $0) }
        let m = try XCTUnwrap(AIChatMaterial.request(entity: try entity(), records: long, uid: "me", limit: nil))
        XCTAssertLessThanOrEqual(m.text.utf8.count, AIChatMaterial.maxBytes)
        XCTAssertTrue(m.text.hasPrefix("- 本文: " + String(repeating: "あ", count: 1000) + "20"), "新しい記録から入れる")
        XCTAssertFalse(m.text.contains("あ1 /"), "入りきらない古い記録は落とす")
        // 1 件で 8 KB を超える記録は切って入れる
        let huge = [rec("h", String(repeating: "い", count: 5000), day: 1)]
        let one = try XCTUnwrap(AIChatMaterial.request(entity: try entity(), records: huge, uid: "me", limit: nil))
        XCTAssertLessThanOrEqual(one.text.utf8.count, AIChatMaterial.maxBytes)
        XCTAssertGreaterThan(one.text.utf8.count, AIChatMaterial.maxBytes - 4)
    }

    func testNameMatchesWhatTheServerExpects() throws {
        let m = try XCTUnwrap(AIChatMaterial.request(entity: try entity(name: "  日記  "), records: [rec("a", "x", day: 1)], uid: nil, limit: nil))
        XCTAssertEqual(m.name, "日記")
        let unnamed = try XCTUnwrap(AIChatMaterial.request(entity: try entity(name: " "), records: [rec("a", "x", day: 1)], uid: nil, limit: nil))
        XCTAssertNil(unnamed.name)
    }
}
