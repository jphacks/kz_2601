import XCTest
@testable import MetamonDSL

/// #270: `{"myFollows": true}` の filter と follow / unfollow のアクション。
final class FollowFilterTests: XCTestCase {
    private let posts: [Record] = [
        Record(id: "1", data: ["body": "a"], author: "alice"),
        Record(id: "2", data: ["body": "b"], author: "bob"),
        Record(id: "3", data: ["body": "c"], author: "me"),
        Record(id: "4", data: ["body": "d"]), // 作者の無い自分の未送信
    ]

    func testTimelineExampleDecodes() throws {
        let dsl = try ExamplesFixture.load("timeline")
        XCTAssertTrue(dsl.usesFollows)
        let list = try XCTUnwrap(dsl.screen("following")?.allComponents.first { $0.type == .list })
        XCTAssertEqual(list.filters.count, 1, "{\"myFollows\":true} が Expr として捨てられない")
        XCTAssertTrue(list.filters[0].isMyFollows)
        let buttons = try XCTUnwrap(dsl.screen("post_form")).allComponents.compactMap(\.action?.type)
        XCTAssertTrue(buttons.contains(.follow))
        XCTAssertTrue(buttons.contains(.unfollow))
        XCTAssertFalse(try ExamplesFixture.load("board").usesFollows)
    }

    func testResolvedFilterKeepsFollowedAuthors() throws {
        let f = Filter(field: RecordValue.author, op: "eq", value: Filter.myFollowsValue)
        let ids = { (f: Filter) in self.posts.filter { Aggregator.matches(f, record: $0, field: nil) }.map(\.id) }
        XCTAssertEqual(ids(f), [], "読み込む前はどの行にも一致しない")
        XCTAssertEqual(ids(Filter(field: RecordValue.author, op: "neq", value: Filter.myFollowsValue)), [])

        let resolved = f.resolvingFollows(["alice"])
        XCTAssertEqual(resolved.followedAuthors, ["alice"])
        XCTAssertEqual(ids(resolved), ["1"])
        var neq = resolved
        neq.op = "neq"
        XCTAssertEqual(ids(neq), ["2", "3", "4"], "フォローしていない人 = 自分も含む")
        XCTAssertEqual(ids(f.resolvingFollows([])), [], "誰もフォローしていない")
    }

    func testOnlyMyFollowsIsResolved() {
        let me = Filter(field: RecordValue.author, op: "eq", value: Filter.meValue)
        XCTAssertEqual(me.resolvingFollows(["alice"]), me)
        XCTAssertNil(me.followedAuthors)
        XCTAssertNil(Filter(field: "tags", op: "eq", value: .array([.string("x")])).followedAuthors)
    }

    func testActionTypesDecode() throws {
        let a = try JSONDecoder().decode(Action.self, from: Data(#"{"type":"unfollow","entity":"posts"}"#.utf8))
        XCTAssertEqual(a.type, .unfollow)
        XCTAssertEqual(a.entity, "posts")
        XCTAssertTrue(a.type.isFollow)
    }
}
