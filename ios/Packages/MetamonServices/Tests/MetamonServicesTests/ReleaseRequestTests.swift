import XCTest
@testable import MetamonServices

final class ReleaseRequestTests: XCTestCase {
    func testDecodeAndStatus() throws {
        let json = #"{"id":"rr_1","appId":"a","name":"家計簿","dslVersion":3,"purpose":"毎日","contact":"","status":"planned","createdAt":"2026-10-24T00:00:00Z","updatedAt":"2026-10-24T00:00:00Z"}"#
        let r = try JSONDecoder().decode(ReleaseRequest.self, from: Data(json.utf8))
        XCTAssertEqual(r.kind, .planned)
        XCTAssertEqual(r.dslVersion, 3)
        // コンソールの打ち間違いは受付扱い
        XCTAssertEqual(ReleaseRequest(id: "x", appId: "a", status: "Reviewing ").kind, .received)
    }

    func testLatestByAppPrefersOpen() {
        // 新しい順: a は見送り後に再申請(受付)、b は見送りのみ
        let items = [
            ReleaseRequest(id: "3", appId: "a", status: "received"),
            ReleaseRequest(id: "2", appId: "b", status: "declined"),
            ReleaseRequest(id: "1", appId: "a", status: "declined"),
        ]
        let m = ReleaseRequest.latestByApp(items)
        XCTAssertEqual(m["a"]?.id, "3")
        XCTAssertEqual(m["b"]?.kind, .declined)
        // 古い申請中 + 新しい見送り(運用上まれ)でも申請中を出す
        let m2 = ReleaseRequest.latestByApp([ReleaseRequest(id: "2", appId: "a", status: "declined"),
                                             ReleaseRequest(id: "1", appId: "a", status: "reviewing")])
        XCTAssertEqual(m2["a"]?.kind, .reviewing)
    }
}
