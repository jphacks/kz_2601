import XCTest
@testable import MetamonRenderer

/// 画面を消さない: 持っている所が 1 つでもあれば消さない(文字起こしの終わりでドラレコ・道案内の分まで戻さない)。
@MainActor
final class ScreenAwakeTests: XCTestCase {
    func testStaysOnWhileAnyHolderRemains() {
        var applied: [Bool] = []
        let awake = ScreenAwake { applied.append($0) }
        let dashcam = awake.hold()
        let transcribe = awake.hold()
        XCTAssertEqual(applied, [true])
        transcribe.release()
        XCTAssertEqual(applied, [true], "ドラレコが持っている間は戻さない")
        XCTAssertEqual(awake.holders, 1)
        dashcam.release()
        XCTAssertEqual(applied, [true, false])
    }

    func testReleaseIsIdempotent() {
        var applied: [Bool] = []
        let awake = ScreenAwake { applied.append($0) }
        let a = awake.hold()
        let b = awake.hold()
        a.release()
        a.release()
        XCTAssertEqual(awake.holders, 1, "2 回目の release で他の持ち主の分を減らさない")
        b.release()
        XCTAssertEqual(applied, [true, false])
        XCTAssertEqual(awake.holders, 0)
    }
}
