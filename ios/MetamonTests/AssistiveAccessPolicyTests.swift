import XCTest
import MetamonServices
@testable import Metamon

/// アシスティブアクセス(#1997): 出す・出さないと最初の画面の決まり。
final class AssistiveAccessPolicyTests: XCTestCase {
    private let active = AssistiveAccessPolicy(isActive: true)

    // MARK: - 出す・出さない

    func testStandardShowsEverything() {
        for feature in AssistiveAccessPolicy.Feature.allCases {
            XCTAssertTrue(AssistiveAccessPolicy.standard.shows(feature), "\(feature)")
        }
    }

    func testAssistiveAccessHidesCreateAndPaidFeatures() {
        for feature in AssistiveAccessPolicy.Feature.allCases {
            XCTAssertFalse(active.shows(feature), "\(feature)")
        }
    }

    // MARK: - 最初の画面

    func testStandardHomeIsUnchanged() {
        XCTAssertEqual(AssistiveAccessPolicy.standard.home(openedAppId: nil, currentAppId: "a"), .standard)
        XCTAssertEqual(AssistiveAccessPolicy.standard.home(openedAppId: "a", currentAppId: "a"), .standard)
    }

    func testAssistiveAccessStartsWithAppList() {
        XCTAssertEqual(active.home(openedAppId: nil, currentAppId: "a"), .appList)
    }

    func testOpenedAppShowsCurrentScreen() {
        XCTAssertEqual(active.home(openedAppId: "a", currentAppId: "a"), .openedApp)
    }

    /// 切り替えに失敗して今のアプリが別のまま → 一覧に留まる
    func testOpenedButNotSwitchedStaysOnList() {
        XCTAssertEqual(active.home(openedAppId: "b", currentAppId: "a"), .appList)
    }

    // MARK: - 一覧に出すアプリ

    func testListedAppsDropBlankAndArchivedKeepingOrder() {
        let apps = [
            AppSummary(id: "blank", name: "初期", currentVersion: 1),
            AppSummary(id: "b", name: "家計簿", currentVersion: 3),
            AppSummary(id: "old", name: "古い", currentVersion: 4, status: "archived"),
            AppSummary(id: "a", name: "日記", currentVersion: 2),
        ]
        XCTAssertEqual(AssistiveAccessPolicy.listedApps(apps).map(\.id), ["b", "a"])
    }

    // MARK: - 判定

    func testDetect() {
        XCTAssertFalse(AssistiveAccessPolicy.detect(systemEnabled: false, debugOverride: nil).isActive)
        XCTAssertTrue(AssistiveAccessPolicy.detect(systemEnabled: true, debugOverride: nil).isActive)
        XCTAssertTrue(AssistiveAccessPolicy.detect(systemEnabled: false, debugOverride: "1").isActive)
        XCTAssertFalse(AssistiveAccessPolicy.detect(systemEnabled: false, debugOverride: "0").isActive)
    }
}
