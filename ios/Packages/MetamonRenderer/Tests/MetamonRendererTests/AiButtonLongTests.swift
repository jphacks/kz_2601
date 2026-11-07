import XCTest
import MetamonDSL
@testable import MetamonRenderer

/// aiButton `length: long`: いつもオンライン・材料 16 KB・依頼に length を付ける・長い答えは段落を残す。
final class AiButtonLongTests: XCTestCase {
    private func component(_ json: String) throws -> Component {
        try JSONDecoder().decode(Component.self, from: Data(json.utf8))
    }

    func testLongAlwaysOnline() {
        XCTAssertEqual(AIGenerateRoute.choose(availability: .available, hostOnline: true, mock: false, long: true), .online,
                       "端末内 AI があっても長い文はオンライン")
        XCTAssertEqual(AIGenerateRoute.choose(availability: .available, hostOnline: false, mock: false, long: true),
                       .unavailable(AIGenerateRoute.longUnavailable), "オンラインの無いホスト(審査用など)は理由を出して無効")
        XCTAssertEqual(AIGenerateRoute.choose(availability: .available, hostOnline: true, mock: false, long: false), .onDevice)
    }

    func testGenerateLongRequest() throws {
        let c = try component(#"{"type":"aiButton","id":"letter","task":"generate","prompt":"{{body}} への手紙","input":"body","length":"long"}"#)
        let entity = try JSONDecoder().decode(Entity.self, from: Data(#"{"id":"d","name":"日記","fields":[{"id":"body","name":"本文","type":"text"}]}"#.utf8))
        XCTAssertEqual(c.length, "long")
        let big = String(repeating: "あ", count: 6000) // 18000 bytes
        let r = try AIGenerateBuilder.build(c, form: entity, values: ["body": .string(big)], sourceEntity: nil, records: [], locale: "ja")
        XCTAssertTrue(r.isLong)
        XCTAssertEqual(r.length, "long")
        XCTAssertEqual(r.shape, .line)
        XCTAssertLessThanOrEqual(r.input?.utf8.count ?? 0, AITextLimits.maxLongInputBytes)
        XCTAssertGreaterThan(r.input?.utf8.count ?? 0, AITextLimits.maxInputBytes, "4 KB では切らない")

        let short = try component(#"{"type":"aiButton","task":"generate","prompt":"一言","input":"body"}"#)
        let s = try AIGenerateBuilder.build(short, form: entity, values: ["body": .string(big)], sourceEntity: nil, records: [], locale: "ja")
        XCTAssertNil(s.length)
        XCTAssertLessThanOrEqual(s.input?.utf8.count ?? 0, AITextLimits.maxInputBytes)
    }

    func testSummarizeLongRequestButNotClassify() throws {
        let entity = try JSONDecoder().decode(Entity.self, from: Data(
            #"{"id":"d","name":"規約","fields":[{"id":"body","name":"本文","type":"text"},{"id":"risks","name":"不利な点","type":"text"}]}"#.utf8))
        let c = try component(#"{"type":"aiButton","task":"extract","input":"body","output":"risks","length":"long"}"#)
        let r = try AIRequestBuilder.online(c, entity: entity, values: ["body": .string(String(repeating: "規約", count: 3000))], locale: "ja")
        XCTAssertEqual(r.length, "long")
        XCTAssertGreaterThan(r.input?.utf8.count ?? 0, AITextLimits.maxInputBytes)
        XCTAssertLessThanOrEqual(r.input?.utf8.count ?? 0, AITextLimits.maxLongInputBytes)
    }
}
