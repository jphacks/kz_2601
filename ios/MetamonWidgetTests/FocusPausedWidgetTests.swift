import SwiftUI
import WidgetKit
import XCTest

/// #1992: 集中モードのフィルタで選んでいない生成アプリは、数字の代わりに「集中モード中」を描く。
@MainActor
final class FocusPausedWidgetTests: XCTestCase {
    private func render(_ content: some View, size: CGSize) throws -> UIImage {
        let renderer = ImageRenderer(content: content.frame(width: size.width, height: size.height))
        renderer.scale = 3
        return try XCTUnwrap(renderer.uiImage)
    }

    func testRendersEveryFamily() throws {
        let cases: [(WidgetFamily, CGSize)] = [
            (.systemSmall, CGSize(width: 158, height: 158)),
            (.systemMedium, CGSize(width: 338, height: 158)),
            (.systemLarge, CGSize(width: 338, height: 354)),
            (.accessoryRectangular, CGSize(width: 160, height: 72)),
            (.accessoryCircular, CGSize(width: 72, height: 72)),
            (.accessoryInline, CGSize(width: 234, height: 26)),
        ]
        for (family, size) in cases {
            let image = try render(FocusPausedView(family: family), size: size)
            XCTAssertEqual(image.size, size, "\(family)")
        }
    }

    /// 数字の View を通らない(選んでいないアプリの数字を出さない)。
    func testPausedEntryRendersWithoutStats() throws {
        var snapshot = WidgetSnapshot.sample
        snapshot.appId = "b"
        let entry = StatEntry(date: .now, snapshot: snapshot, pausedByFocus: true)
        let image = try render(MetamonWidgetView(entry: entry), size: CGSize(width: 158, height: 158))
        XCTAssertEqual(image.size.width, 158)
        XCTAssertTrue(FocusFilterState.pausesWidget(snapshot, focus: FocusFilterState(appIds: ["a"], silencesNotifications: false)))
    }
}
