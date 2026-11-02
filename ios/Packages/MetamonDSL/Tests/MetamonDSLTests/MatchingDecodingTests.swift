#if !APP_STORE_REVIEW
import XCTest
@testable import MetamonDSL

/// #507 phase 2: 仲間探しの entity.match・list.item.interest・matchList・API の応答のデコード。
final class MatchingDecodingTests: XCTestCase {
    /// backend の Validate を通る例(backend/pkg/dsl/testdata/matching.json)をそのまま読む。
    func testBackendExampleDecodes() throws {
        let data = try Data(contentsOf: ExamplesFixture.repoRoot.appending(path: "backend/pkg/dsl/testdata/matching.json"))
        let dsl = try JSONDecoder().decode(AppDSL.self, from: data)
        let profile = try XCTUnwrap(dsl.entity("profile"))
        XCTAssertEqual(profile.match, "profile")
        XCTAssertTrue(profile.isMatchProfile)
        XCTAssertFalse(profile.isMatchMatched)
        XCTAssertTrue(try XCTUnwrap(dsl.entity("contact")).isMatchMatched)

        let list = try XCTUnwrap(dsl.screen("home")?.allComponents.first { $0.entity == "profile" })
        XCTAssertEqual(list.item?.interest, true)
        let lists = try XCTUnwrap(dsl.screen("connections")).allComponents.filter { $0.type == .matchList }
        XCTAssertEqual(lists.map(\.show), ["incoming", "matches"])
        XCTAssertEqual(lists.compactMap { MatchBox(show: $0.show) }, [.incoming, .matches])
        XCTAssertEqual(lists[0].item?.title, "hobby")
        XCTAssertEqual(dsl.entitiesNeeded(by: try XCTUnwrap(dsl.screen("connections"))), ["profile"], "行を描くプロフィールを読む")
    }

    /// private の entity に match があっても仲間探しにしない・未知の値は無視。
    func testMatchNeedsSharedScope() throws {
        let json = #"{"id":"p","name":"p","match":"profile","fields":[]}"#
        let e = try JSONDecoder().decode(Entity.self, from: Data(json.utf8))
        XCTAssertFalse(e.isMatchProfile)
        let odd = try JSONDecoder().decode(Entity.self, from: Data(#"{"id":"p","name":"p","scope":"public","match":"dating","fields":[]}"#.utf8))
        XCTAssertFalse(odd.isMatchProfile)
        XCTAssertFalse(odd.isMatchMatched)
        XCTAssertNil(MatchBox(show: "likes"))
        XCTAssertNil(MatchBox(show: nil))
    }

    /// appconfig `interestOut`(Go の time.Time はナノ秒まで・matchedAt は matched だけ)。
    func testInterestDecodes() throws {
        let json = #"""
        {"items":[
          {"uid":"u2","profileRecordId":"r2","status":"matched","updatedAt":"2026-11-02T10:00:00.123456789Z","matchedAt":"2026-11-02T10:00:00.123456789Z"},
          {"uid":"u3","status":"pending","updatedAt":"2026-11-02T09:00:00Z"},
          {"uid":"u4"}
        ]}
        """#
        struct Box: Decodable { var items: [MatchInterest] }
        let items = try JSONDecoder().decode(Box.self, from: Data(json.utf8)).items
        XCTAssertEqual(items.map(\.uid), ["u2", "u3", "u4"])
        XCTAssertTrue(items[0].isMatched)
        XCTAssertEqual(items[0].profileRecordId, "r2")
        XCTAssertNil(items[1].profileRecordId)
        XCTAssertFalse(items[1].isMatched)
        XCTAssertEqual(items[2].status, "pending", "status が無ければ pending")
    }
}
#endif
