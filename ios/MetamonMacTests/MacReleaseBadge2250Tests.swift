import XCTest
import MetamonServices
@testable import Metamon

/// サイドバーの行のリリース申請の状況の札(#2250): 4 つの状況の文言と読み上げ。
/// 文言はテストの言語(ja / en)によらず、カタログの語(iOS と同じキー)で比べる
@MainActor
final class MacReleaseBadge2250Tests: XCTestCase {
    func testLabelsMatchIOSForAllStatuses() {
        let expected: [ReleaseStatus: (String, String)] = [
            .received: (L10n.tr("受付"), "tray.and.arrow.down"),
            .reviewing: (L10n.tr("検討中"), "magnifyingglass"),
            .planned: (L10n.tr("リリース予定"), "shippingbox"),
            .declined: (L10n.tr("見送り"), "xmark.circle"),
        ]
        for status in ReleaseStatus.allCases {
            let badge = MacSidebarReleaseBadge(status)
            XCTAssertEqual(badge.label, expected[status]?.0, "\(status)")
            XCTAssertEqual(badge.symbol, expected[status]?.1, "\(status)")
            XCTAssertEqual(badge.accessibilityLabel, L10n.tr("リリース申請: \(expected[status]?.0 ?? "")"), "\(status)")
            XCTAssertNotEqual(badge.label, "", "\(status)")
            XCTAssertEqual(badge.dimmed, status == .declined, "見送りだけ薄く")
        }
    }

    func testRowAccessibilityLabelIncludesStatus() {
        let badge = MacSidebarReleaseBadge(.reviewing)
        XCTAssertEqual(MacSidebarBadges.accessibilityLabel(name: "家計簿", count: 0, release: badge), "家計簿, \(badge.accessibilityLabel)")
        let both = MacSidebarBadges.accessibilityLabel(name: "家計簿", count: 2, release: badge)
        XCTAssertTrue(both.hasPrefix("家計簿"), both)
        XCTAssertTrue(both.contains("2"), "新着の数も読む")
        XCTAssertTrue(both.hasSuffix(badge.accessibilityLabel), both)
        XCTAssertEqual(MacSidebarBadges.accessibilityLabel(name: "家計簿", count: 0, release: nil), "家計簿", "申請していなければ今まで通り")
    }

    func testBadgeFollowsLatestRequest() {
        let byApp = ReleaseRequest.latestByApp([ReleaseRequest(id: "r2", appId: "a", status: "planned"),
                                                ReleaseRequest(id: "r1", appId: "a", status: "declined")])
        XCTAssertEqual(MacSidebarReleaseBadge.badge(appId: "a", in: byApp)?.label, L10n.tr("リリース予定"))
        XCTAssertNil(MacSidebarReleaseBadge.badge(appId: "b", in: byApp), "申請していないアプリは出さない")
    }

    func testStoreChangeUpdatesBadge() async {
        let store = MacReleaseRequests()
        store.record(ReleaseRequest(id: "r1", appId: "a", status: "received"))
        XCTAssertEqual(MacSidebarReleaseBadge.badge(appId: "a", in: store.byApp)?.label, L10n.tr("受付"))
        store.record(ReleaseRequest(id: "r1", appId: "a", status: "reviewing"))
        XCTAssertEqual(MacSidebarReleaseBadge.badge(appId: "a", in: store.byApp)?.label, L10n.tr("検討中"), "状況が変わると変わる")
        store.clear()
        XCTAssertNil(MacSidebarReleaseBadge.badge(appId: "a", in: store.byApp), "サインアウトで消える")
    }
}
