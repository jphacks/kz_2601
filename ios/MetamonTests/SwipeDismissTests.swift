import XCTest
import SwiftUI
@testable import Metamon

/// #628: 上(トーストは下)の通知をスワイプで閉じる判定。
final class SwipeDismissTests: XCTestCase {
    func testTopBannerClosesWhenSwipedUpFarEnough() {
        XCTAssertTrue(SwipeDismiss.shouldDismiss(translation: -SwipeDismiss.distance, predicted: -SwipeDismiss.distance, edge: .top))
        XCTAssertTrue(SwipeDismiss.shouldDismiss(translation: -80, predicted: -90, edge: .top))
    }

    /// 途中で離したら戻る(閉じない)。
    func testShortSwipeSpringsBack() {
        XCTAssertFalse(SwipeDismiss.shouldDismiss(translation: -(SwipeDismiss.distance - 1), predicted: -40, edge: .top))
    }

    /// 短くても速く払えば閉じる。
    func testFlingCloses() {
        XCTAssertTrue(SwipeDismiss.shouldDismiss(translation: -10, predicted: -SwipeDismiss.flingDistance, edge: .top))
    }

    /// 逆向き(下)へは閉じない・少しだけ動いて止まる。
    func testWrongDirectionNeverCloses() {
        XCTAssertFalse(SwipeDismiss.shouldDismiss(translation: 200, predicted: 400, edge: .top))
        XCTAssertEqual(SwipeDismiss.offset(translation: 200, edge: .top), SwipeDismiss.maxPull)
        XCTAssertEqual(SwipeDismiss.offset(translation: -30, edge: .top), -30)
    }

    /// 下に出るトーストは下に払って閉じる。
    func testBottomToastClosesDownward() {
        XCTAssertTrue(SwipeDismiss.shouldDismiss(translation: 50, predicted: 60, edge: .bottom))
        XCTAssertFalse(SwipeDismiss.shouldDismiss(translation: -50, predicted: -200, edge: .bottom))
        XCTAssertEqual(SwipeDismiss.offset(translation: 30, edge: .bottom), 30)
    }

    func testOpacityFadesOnlyTowardEdge() {
        XCTAssertEqual(SwipeDismiss.opacity(translation: 0, edge: .top), 1)
        XCTAssertEqual(SwipeDismiss.opacity(translation: 50, edge: .top), 1)
        XCTAssertLessThan(SwipeDismiss.opacity(translation: -50, edge: .top), 1)
        XCTAssertGreaterThanOrEqual(SwipeDismiss.opacity(translation: -1000, edge: .top), 0.4)
    }
}
