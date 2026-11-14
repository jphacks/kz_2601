import XCTest
@testable import MetamonServices

/// 断られた書き込みの宛先(iOS / Mac のランタイムが「保存しました」の取り消し・aiChat の対の片付けに使う)。
final class SyncEventRefusedWriteTests: XCTestCase {
    func testRefusalsCarryTarget() {
        let refused: [SyncEvent] = [
            .rejected(appId: "a", entity: "e", message: "x", recordId: "local-1"),
            .forbidden(appId: "a", entity: "e", recordId: "local-1"),
            .notMember(appId: "a", entity: "e", recordId: "local-1"),
            .denied(appId: "a", entity: "e", denial: .adminsOnly(field: nil), recordId: "local-1"),
            .planLimit(appId: "a", entity: "e", limit: PlanLimit(resource: .recordsPerApp), recordId: "local-1"),
            .appStopped(appId: "a", entity: "e", recordId: "local-1"),
            .textNotAllowed(appId: "a", entity: "e", field: nil, recordId: "local-1"),
        ]
        for event in refused {
            let target = event.refusedWrite
            XCTAssertEqual(target?.appId, "a", "\(event)")
            XCTAssertEqual(target?.entity, "e", "\(event)")
            XCTAssertEqual(target?.recordId, "local-1", "\(event)")
        }
        XCTAssertNil(SyncEvent.rejected(appId: "a", entity: "e", message: "x").refusedWrite?.recordId)
    }

    func testNonRefusalsAreNil() {
        XCTAssertNil(SyncEvent.sent(appId: "a", entity: "e").refusedWrite)
        XCTAssertNil(SyncEvent.conflict(appId: "a", entity: "e", message: "x").refusedWrite)
        XCTAssertNil(SyncEvent.paused(pending: 1).refusedWrite)
        XCTAssertNil(SyncEvent.drained.refusedWrite)
    }
}
