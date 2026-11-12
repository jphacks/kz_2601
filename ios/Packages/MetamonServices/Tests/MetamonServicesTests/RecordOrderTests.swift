import XCTest
import MetamonDSL
@testable import MetamonServices

/// RecordStore.rows の並び: createdAt を時刻で比べる・同時刻は後から入った行が先・サーバーの同時刻の並びを保つ。
final class RecordOrderTests: XCTestCase {
    private var dir: URL!
    private var store: RecordStore!

    override func setUp() {
        dir = FileManager.default.temporaryDirectory.appendingPathComponent("metamon-order-\(UUID().uuidString)")
        store = RecordStore(directory: dir)
    }

    override func tearDown() {
        try? FileManager.default.removeItem(at: dir)
    }

    private func row(_ id: String, _ createdAt: String?) -> LocalRecord {
        LocalRecord(appId: "a", entity: "e", recordId: id, data: [:], createdAt: createdAt)
    }

    private func ids() async -> [String] {
        await store.rows(appId: "a", entity: "e").map(\.recordId)
    }

    func testMixedFractionalPrecisionAndOffsetsSortByTime() async {
        // 文字列で比べると ".5Z" や ".123456Z" が "01Z" より後(=新しい)扱いになり、時刻とずれる
        await store.upsert(row("ms", "2026-11-12T10:00:00.123Z"))
        await store.upsert(row("us", "2026-11-12T10:00:00.123456Z"))
        await store.upsert(row("none", "2026-11-12T10:00:01Z"))
        await store.upsert(row("half", "2026-11-12T10:00:00.5Z"))
        await store.upsert(row("offset", "2026-11-12T19:00:02+09:00"))
        await store.upsert(row("ns", "2026-11-12T10:00:00.123000001Z"))
        let got = await ids()
        XCTAssertEqual(got, ["offset", "none", "half", "us", "ns", "ms"])
    }

    func testEqualTimestampsNewestInsertFirstAndStable() async {
        let t = "2026-11-12T10:00:00Z"
        for id in ["local-c", "local-a", "local-b"] { await store.upsert(row(id, t)) }
        var got = await ids()
        XCTAssertEqual(got, ["local-b", "local-a", "local-c"])
        // 同じ瞬間の別表記(小数秒の 0・オフセット)も同時刻として扱う
        await store.upsert(row("local-d", "2026-11-12T19:00:00.000+09:00"))
        got = await ids()
        XCTAssertEqual(got, ["local-d", "local-b", "local-a", "local-c"])
        // 更新しても位置は変わらない
        var updated = row("local-c", t)
        updated.updatedAt = "2026-11-12T11:00:00Z"
        await store.upsert(updated)
        got = await ids()
        XCTAssertEqual(got, ["local-d", "local-b", "local-a", "local-c"])
    }

    func testMissingOrInvalidCreatedAtGoesLast() async {
        await store.upsert(row("nil", nil))
        await store.upsert(row("new", "2026-11-12T10:00:00Z"))
        await store.upsert(row("broken", "yesterday"))
        await store.upsert(row("empty", ""))
        await store.upsert(row("old", "1970-01-01T00:00:00Z"))
        let got = await ids()
        // 読めないもの同士は後から入った行が先
        XCTAssertEqual(got, ["new", "old", "empty", "broken", "nil"])
    }

    func testReplaceKeepsServerOrderForTies() async {
        let t = "2026-11-12T10:00:00Z"
        // サーバー(memory.go / Firestore)は createdAt 降順・同時刻は id 降順で返す
        let server = [
            Record(id: "z", data: [:], createdAt: "2026-11-12T10:00:01Z"),
            Record(id: "c", data: [:], createdAt: t),
            Record(id: "b", data: [:], createdAt: t),
            Record(id: "a", data: [:], createdAt: "2026-11-12T09:59:59.999999Z"),
        ]
        await store.replace(appId: "a", entity: "e", with: server)
        var got = await ids()
        XCTAssertEqual(got, ["z", "c", "b", "a"])
        // その後に同時刻で作った未送信の行は同時刻の先頭
        await store.upsert(row("local-new", t))
        got = await ids()
        XCTAssertEqual(got, ["z", "local-new", "c", "b", "a"])
    }
}
