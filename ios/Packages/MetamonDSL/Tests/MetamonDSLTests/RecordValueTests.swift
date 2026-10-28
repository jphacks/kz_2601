import XCTest
@testable import MetamonDSL

/// #250 #251: 予約値 `$author` / `$createdAt` と「自分の投稿だけ」の filter。
final class RecordValueTests: XCTestCase {
    private let posts: [Record] = [
        Record(id: "1", data: ["title": "傘"], createdAt: "2026-10-26T09:00:00Z", author: "me", authorName: "さくら"),
        Record(id: "2", data: ["title": "鍵"], createdAt: "2026-10-26T10:00:00Z", author: "other", authorName: "匿名"),
        Record(id: "3", data: ["title": "財布"], createdAt: "2026-10-26T11:00:00Z"), // 作者の無い自分の未送信
    ]

    func testSchemaRecordValuesMatchIOS() throws {
        let data = try Data(contentsOf: ExamplesFixture.schemaURL)
        let root = try XCTUnwrap(try JSONSerialization.jsonObject(with: data) as? [String: Any])
        let entity = try XCTUnwrap((root["$defs"] as? [String: Any])?["Entity"] as? [String: Any])
        let values = try XCTUnwrap(entity["x-recordValues"] as? [String: Any])
        XCTAssertEqual(Set(values.keys), RecordValue.all, "schema の x-recordValues と RecordValue がずれている")
    }

    func testBoardExampleDecodesRecordValuesAndMeFilter() throws {
        let dsl = try ExamplesFixture.load("board")
        let entity = try XCTUnwrap(dsl.entity("items"))
        XCTAssertTrue(entity.isShared)
        let list = try XCTUnwrap(dsl.screen("home")?.allComponents.first { $0.type == .list })
        XCTAssertEqual(list.item?.subtitle, RecordValue.author)
        XCTAssertEqual(list.sort?.field, RecordValue.createdAt)
        let stat = try XCTUnwrap(dsl.screen("mine")?.allComponents.first { $0.type == .stat })
        XCTAssertEqual(stat.filters.count, 1, "{\"me\":true} が Expr として捨てられない")
        XCTAssertTrue(stat.filters[0].isMe)
    }

    func testRecordSubscriptAndVirtualFields() throws {
        XCTAssertEqual(posts[0][RecordValue.author], "さくら")
        XCTAssertEqual(posts[0][RecordValue.createdAt], "2026-10-26T09:00:00Z")
        XCTAssertEqual(posts[2][RecordValue.author], .null)

        let dsl = try ExamplesFixture.load("board")
        let shared = try XCTUnwrap(dsl.entity("items"))
        XCTAssertEqual(shared.field(RecordValue.author)?.type, .string)
        XCTAssertEqual(shared.field(RecordValue.createdAt)?.type, .datetime)
        XCTAssertNil(shared.storedFields.first { $0.id.hasPrefix("$") }, "保存フィールドには入らない")
        let kakeibo = try ExamplesFixture.load("kakeibo")
        let priv = try XCTUnwrap(kakeibo.entities.first)
        XCTAssertNil(priv.field(RecordValue.author), "private の entity に投稿者は無い")
        XCTAssertNotNil(priv.field(RecordValue.createdAt))
    }

    func testMeFilterComparesAuthorUID() {
        let me = Filter(field: RecordValue.author, op: "eq", value: Filter.meValue)
        XCTAssertTrue(me.isMe)
        // uid に置き換える前の me はどれにも一致しない(ウィジェット等 uid の無い経路)
        XCTAssertTrue(Aggregator.apply(me, entity: nil, to: posts).isEmpty)

        let mine = Aggregator.apply(me.resolvingMe(uid: "me"), entity: nil, to: posts)
        XCTAssertEqual(mine.map(\.id), ["1"])
        let others = Aggregator.apply(Filter(field: RecordValue.author, op: "neq", value: Filter.meValue).resolvingMe(uid: "me"), entity: nil, to: posts)
        XCTAssertEqual(others.map(\.id), ["2", "3"])
        // サインインしていない時は作者の無い行(自分の未送信)だけ
        XCTAssertEqual(Aggregator.apply(me.resolvingMe(uid: nil), entity: nil, to: posts).map(\.id), ["3"])
    }

    func testSortByCreatedAt() {
        let sorted = Aggregator.sorted(posts, by: Sort(field: RecordValue.createdAt, direction: "desc"), entity: nil)
        XCTAssertEqual(sorted.map(\.id), ["3", "2", "1"])
    }

    func testMeFilterRoundTrips() throws {
        let json = #"{"field":"$author","op":"eq","value":{"me":true}}"#
        let f = try JSONDecoder().decode(Filter.self, from: Data(json.utf8))
        XCTAssertTrue(f.isMe)
        XCTAssertNil(f.valueExpr)
        let again = try JSONDecoder().decode(Filter.self, from: try JSONEncoder().encode(f))
        XCTAssertTrue(again.isMe)
        // me: false は「自分」ではない(Expr でもないので filter ごと読めない = 絞り込み無し)
        XCTAssertThrowsError(try JSONDecoder().decode(Filter.self, from: Data(#"{"field":"$author","op":"eq","value":{"me":false}}"#.utf8)))
    }
}
