import XCTest
import UIKit
import MetamonServices
@testable import Metamon

/// ホーム画面の長押しメニュー(#2232): 項目の並び(最近の順・消したアプリが消える・上限 4)と押された時の行き先。
@MainActor
final class HomeQuickActions2232Tests: XCTestCase {
    private func app(_ id: String, _ name: String, status: String = "active") -> AppSummary {
        AppSummary(id: id, name: name, currentVersion: 2, status: status, updatedAt: "2026-11-01T00:00:00Z")
    }

    private var apps: [AppSummary] {
        [app("a", "家計簿"), app("b", "読書記録"), app("c", "体重"), app("d", "献立"), app("old", "昔の", status: "archived")]
    }

    func testRecentOrderAndTalkLast() {
        let items = QuickActionItem.items(recent: ["c", "a", "b", "d"], apps: apps)
        XCTAssertEqual(items, [.app(id: "c", name: "体重"), .app(id: "a", name: "家計簿"), .app(id: "b", name: "読書記録"), .talk])
    }

    func testDeletedAndArchivedAppsDisappear() {
        let items = QuickActionItem.items(recent: ["gone", "old", "b"], apps: apps)
        XCTAssertEqual(items, [.app(id: "b", name: "読書記録"), .talk])
    }

    func testCappedAtFour() {
        let many = (0..<10).map { app("x\($0)", "アプリ\($0)") }
        let items = QuickActionItem.items(recent: many.map(\.id), apps: many)
        XCTAssertEqual(items.count, QuickActionItem.maxItems)
        XCTAssertEqual(items.last, .talk)
    }

    func testNothingOpenedYetShowsOnlyTalk() {
        XCTAssertEqual(QuickActionItem.items(recent: [], apps: apps), [.talk])
        XCTAssertEqual(QuickActionItem.items(recent: ["a"], apps: []), [.talk])
    }

    func testDuplicatesAndBlankNamesSkipped() {
        let items = QuickActionItem.items(recent: ["a", "a", "blank", "b"], apps: apps + [app("blank", "  ")])
        XCTAssertEqual(items, [.app(id: "a", name: "家計簿"), .app(id: "b", name: "読書記録"), .talk])
    }

    func testRecentStoreMovesToFrontAndCaps() throws {
        let suite = "HomeQuickActions2232Tests"
        let defaults = try XCTUnwrap(UserDefaults(suiteName: suite))
        defer { defaults.removePersistentDomain(forName: suite) }
        let recent = RecentApps(defaults: defaults)
        recent.noteOpened("a")
        recent.noteOpened("b")
        recent.noteOpened("a")
        XCTAssertEqual(recent.ids, ["a", "b"])
        recent.forget("a")
        XCTAssertEqual(recent.ids, ["b"])
        recent.noteOpened("")
        XCTAssertEqual(recent.ids, ["b"])
        let many = (0..<20).reduce([String]()) { RecentApps.noting("id\($1)", in: $0) }
        XCTAssertEqual(many.count, RecentApps.maxStored)
        XCTAssertEqual(many.first, "id19")
    }

    func testRecentKeyIsWipedOnSignOut() {
        XCTAssertFalse(LocalWipe.preservedKeys.contains(RecentApps.key))
    }

    func testShortcutRoundTrip() {
        let open = HomeQuickActions.shortcut(.app(id: "a", name: "家計簿"))
        XCTAssertEqual(open.localizedTitle, "家計簿")
        XCTAssertEqual(HomeQuickActions.request(for: open.type, userInfo: open.userInfo), .control(appId: "a", entityId: nil))
        let talk = HomeQuickActions.shortcut(.talk)
        XCTAssertEqual(HomeQuickActions.request(for: talk.type, userInfo: talk.userInfo), .talk)
        XCTAssertNil(HomeQuickActions.request(for: "unknown", userInfo: nil))
        XCTAssertNil(HomeQuickActions.request(for: HomeQuickActions.openType, userInfo: nil))
    }

    func testAppDelegateAttachesSceneDelegate() {
        let selector = #selector(UIApplicationDelegate.application(_:configurationForConnecting:options:))
        XCTAssertTrue(MetamonAppDelegate().responds(to: selector))
        XCTAssertTrue(QuickActionSceneDelegate().responds(to: #selector(UIWindowSceneDelegate.windowScene(_:performActionFor:completionHandler:))))
    }
}
