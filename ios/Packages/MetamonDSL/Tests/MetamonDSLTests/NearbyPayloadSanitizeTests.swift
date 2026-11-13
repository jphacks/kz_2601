import XCTest
@testable import MetamonDSL

/// 近くの人から受けた記録の取り込み(NearbyShare の sanitize・accepts): 型の合わない値・知らない項目・同じアプリでない時の ref(下書き stock7-ios-011)。
final class NearbyPayloadSanitizeTests: XCTestCase {
    private let json = """
    {"schemaVersion":1,"app":{"name":"t","voiceEntry":"none"},"initialScreen":"home",
     "entities":[{"id":"e","name":"E","fields":[
       {"id":"name","name":"名前","type":"string"},
       {"id":"age","name":"年","type":"integer"},
       {"id":"done","name":"済","type":"boolean"},
       {"id":"owner","name":"持ち主","type":"ref","ref":"e"},
       {"id":"tags","name":"仲間","type":"refs","ref":"e"},
       {"id":"calc","name":"計","type":"integer","computed":{"op":"add","args":[{"ref":"age"},1]}}]}],
     "screens":[{"id":"home","title":"t","components":[]}]}
    """

    private func entity() throws -> Entity {
        let dsl = try JSONDecoder().decode(AppDSL.self, from: Data(json.utf8))
        return try XCTUnwrap(dsl.entity("e"))
    }

    func testAcceptsTableMatchesFieldTypes() {
        let numeric: [FieldType] = [.integer, .number, .currency]
        let stringy: [FieldType] = [.string, .text, .date, .datetime, .enum, .photo, .video, .audio, .location, .ref]
        for t in numeric {
            XCTAssertTrue(NearbyShare.accepts(.number(1), t), "\(t)")
            XCTAssertFalse(NearbyShare.accepts(.string("1"), t), "\(t)")
            XCTAssertFalse(NearbyShare.accepts(.bool(true), t), "\(t)")
        }
        for t in stringy {
            XCTAssertTrue(NearbyShare.accepts(.string("x"), t), "\(t)")
            XCTAssertFalse(NearbyShare.accepts(.number(1), t), "\(t)")
            XCTAssertFalse(NearbyShare.accepts(.array([]), t), "\(t)")
        }
        XCTAssertTrue(NearbyShare.accepts(.bool(false), .boolean))
        XCTAssertFalse(NearbyShare.accepts(.string("true"), .boolean))
        XCTAssertTrue(NearbyShare.accepts(.array([.string("a")]), .refs))
        XCTAssertFalse(NearbyShare.accepts(.string("a"), .refs))
    }

    func testAcceptsRejectsNullAndObjectAndMember() {
        for t in [FieldType.string, .integer, .boolean, .ref, .refs] {
            XCTAssertFalse(NearbyShare.accepts(.null, t), "\(t)")
            XCTAssertFalse(NearbyShare.accepts(.object([:]), t), "\(t)")
        }
        XCTAssertFalse(NearbyShare.accepts(.string("uid"), .member), "member は受けない")
    }

    func testSanitizeDropsUnknownWrongTypeNullAndComputed() throws {
        let e = try entity()
        let rows = NearbyShare.sanitize([[
            "name": .string("花子"), "age": .string("20"), "done": .bool(true),
            "calc": .number(21), "ghost": .string("x"), "owner": .null,
        ]], into: e, sameApp: true)
        XCTAssertEqual(rows, [["name": .string("花子"), "done": .bool(true)]])
    }

    func testSanitizeRefsDependOnSameApp() throws {
        let e = try entity()
        let row: [String: JSONValue] = ["name": .string("a"), "owner": .string("r1"), "tags": .array([.string("r2")])]
        XCTAssertEqual(NearbyShare.sanitize([row], into: e, sameApp: true), [row])
        XCTAssertEqual(NearbyShare.sanitize([row], into: e, sameApp: false), [["name": .string("a")]], "別アプリの ref は相手のレコード id なので捨てる")
    }

    func testSanitizeDropsRowsThatBecomeEmptyAndKeepsOrder() throws {
        let e = try entity()
        let rows = NearbyShare.sanitize([
            ["name": .string("a")],
            ["ghost": .string("x")],
            [:],
            ["owner": .string("r1")],
            ["name": .string("b")],
        ], into: e, sameApp: false)
        XCTAssertEqual(rows, [["name": .string("a")], ["name": .string("b")]])
    }

    func testSanitizeCapsAtMaxReceiveRecordsBeforeFiltering() throws {
        let e = try entity()
        // 上限の件数までを読む(その後ろに有効な行があっても読まない)
        let junk = Array(repeating: ["ghost": JSONValue.string("x")], count: NearbyShare.maxReceiveRecords)
        let rows = NearbyShare.sanitize(junk + [["name": .string("late")]], into: e, sameApp: true)
        XCTAssertEqual(rows, [])
    }

    func testSanitizeEmptyInputGivesEmpty() throws {
        XCTAssertEqual(NearbyShare.sanitize([], into: try entity(), sameApp: true), [])
    }
}
