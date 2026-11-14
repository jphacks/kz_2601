import XCTest
@testable import Metamon

/// 「サブスクリプションを管理」(#326・3.1.2): 通常の版は Pro なら目立つ位置・Pro でなければ復元の近くに出す。
/// 審査用の版(APP_STORE_REVIEW・#450 G1)は App 内課金が無いので出さない。
final class ManageSubscriptionTests: XCTestCase {
    func testPlacement() {
        #if APP_STORE_REVIEW
        XCTAssertNil(ManageSubscriptionPlacement.placement(isPro: true))
        XCTAssertNil(ManageSubscriptionPlacement.placement(isPro: false))
        #else
        XCTAssertEqual(ManageSubscriptionPlacement.placement(isPro: true), .prominent)
        XCTAssertEqual(ManageSubscriptionPlacement.placement(isPro: false), .nearRestore)
        #endif
    }

    #if !APP_STORE_REVIEW
    @MainActor
    func testButtonBuildsInNormalBuild() {
        let button = ManageSubscriptionButton(accessibilityID: "settings.pro.manage")
        XCTAssertEqual(button.accessibilityID, "settings.pro.manage")
    }
    #endif
}
