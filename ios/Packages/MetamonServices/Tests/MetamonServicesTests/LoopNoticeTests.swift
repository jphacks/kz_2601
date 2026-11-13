#if !APP_STORE_REVIEW
import XCTest
@testable import MetamonServices

/// #2846: 直ったお知らせ(#374)の decode の既定。
final class LoopNoticeTests: XCTestCase {
    private func decode<T: Decodable>(_ type: T.Type, _ json: String) throws -> T {
        try JSONDecoder().decode(type, from: Data(json.utf8))
    }

    func testDecodesAllFields() throws {
        let n = try decode(LoopNotice.self, #"{"id":"n1","title":"直りました","body":"ありがとう","createdAt":"2026-11-06T03:00:00Z","readAt":"2026-11-07T04:05:06.789Z"}"#)
        XCTAssertEqual(n.id, "n1")
        XCTAssertEqual(n.title, "直りました")
        XCTAssertEqual(n.body, "ありがとう")
        XCTAssertEqual(n.createdAt, Date(timeIntervalSince1970: 1_793_934_000))
        XCTAssertEqual(try XCTUnwrap(n.readAt).timeIntervalSince1970, 1_794_024_306.789, accuracy: 0.001)
        XCTAssertFalse(n.isUnread)
    }

    func testMissingTitleAndBodyBecomeEmpty() throws {
        let n = try decode(LoopNotice.self, #"{"id":"n2","createdAt":"2026-11-06T03:00:00Z"}"#)
        XCTAssertEqual(n.title, "")
        XCTAssertEqual(n.body, "")
    }

    func testUnreadableCreatedAtIsDistantPast() throws {
        XCTAssertEqual(try decode(LoopNotice.self, #"{"id":"n3","createdAt":"きのう"}"#).createdAt, .distantPast)
        XCTAssertEqual(try decode(LoopNotice.self, #"{"id":"n4"}"#).createdAt, .distantPast)
    }

    func testMissingReadAtIsUnread() throws {
        let n = try decode(LoopNotice.self, #"{"id":"n5","createdAt":"2026-11-06T03:00:00Z"}"#)
        XCTAssertNil(n.readAt)
        XCTAssertTrue(n.isUnread)
    }

    func testUnreadableReadAtIsUnread() throws {
        XCTAssertTrue(try decode(LoopNotice.self, #"{"id":"n6","readAt":"?"}"#).isUnread)
    }

    func testMissingIdThrows() {
        XCTAssertThrowsError(try decode(LoopNotice.self, #"{"title":"x"}"#))
    }

    func testPageDecodesUnreadCount() throws {
        let p = try decode(LoopNoticePage.self, #"{"items":[{"id":"a"},{"id":"b","readAt":"2026-11-07T00:00:00Z"}],"unreadCount":5,"nextPageToken":"t2"}"#)
        XCTAssertEqual(p.items.map(\.id), ["a", "b"])
        XCTAssertEqual(p.items.map(\.isUnread), [true, false])
        // すべてのページの未読の数(このページの未読の数ではない)
        XCTAssertEqual(p.unreadCount, 5)
        XCTAssertEqual(p.nextPageToken, "t2")
    }

    func testPageWithoutNextToken() throws {
        let p = try decode(LoopNoticePage.self, #"{"items":[],"unreadCount":0}"#)
        XCTAssertTrue(p.items.isEmpty)
        XCTAssertEqual(p.unreadCount, 0)
        XCTAssertNil(p.nextPageToken)
    }
}
#endif
