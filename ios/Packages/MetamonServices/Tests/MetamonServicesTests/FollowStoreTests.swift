import XCTest
@testable import MetamonServices

/// #270: フォロー一覧の読み込みと、楽観的な付け外し(失敗したら戻す)。
@MainActor
final class FollowStoreTests: XCTestCase {
    private actor FakeClient: FollowsClient {
        var server: Set<String> = ["alice"]
        var fail = false
        func setFail(_ v: Bool) { fail = v }
        func list(appId: String) async throws -> [String] { Array(server) }
        func setFollow(appId: String, uid: String, on: Bool) async throws -> Bool {
            if fail { throw URLError(.notConnectedToInternet) }
            if on { server.insert(uid) } else { server.remove(uid) }
            return on
        }
    }

    func testLoadAndToggle() async throws {
        let client = FakeClient()
        let store = FollowStore(appId: "A", client: client)
        XCTAssertNil(store.uids, "読む前は nil")
        try await store.load()
        XCTAssertEqual(store.uids, ["alice"])
        try await store.set("bob", on: true)
        XCTAssertTrue(store.isFollowing("bob"))
        try await store.set("alice", on: false)
        XCTAssertEqual(store.uids, ["bob"])
        let server = await client.server
        XCTAssertEqual(server, ["bob"])
    }

    func testFailureRollsBack() async throws {
        let client = FakeClient()
        let store = FollowStore(appId: "A", client: client)
        try await store.load()
        await client.setFail(true)
        do {
            try await store.set("bob", on: true)
            XCTFail("must throw")
        } catch {}
        XCTAssertEqual(store.uids, ["alice"], "失敗したら操作前に戻す")
        XCTAssertTrue(store.busy.isEmpty)
    }
}
