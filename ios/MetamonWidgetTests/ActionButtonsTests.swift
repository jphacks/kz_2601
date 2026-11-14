import SwiftUI
import WidgetKit
import XCTest

/// ボタンを置くウィジェット(#2238)のボタンの並びと見た目。
@MainActor
final class ActionButtonsTests: XCTestCase {
    private let catalog = ControlCatalog(
        apps: [.init(id: "a1", name: "習慣"), .init(id: "a2", name: "家計簿")],
        recordTargets: [
            .init(appId: "a1", entityId: "check", appName: "習慣", entityName: "チェック", showsEntity: true),
            .init(appId: "a1", entityId: "water", appName: "習慣", entityName: "水を飲む", showsEntity: true),
            .init(appId: "a1", entityId: "walk", appName: "習慣", entityName: "散歩", showsEntity: true),
            .init(appId: "a1", entityId: "book", appName: "習慣", entityName: "読書", showsEntity: true),
            .init(appId: "a1", entityId: "sleep", appName: "習慣", entityName: "睡眠", showsEntity: true),
            .init(appId: "a2", entityId: "expense", appName: "家計簿", entityName: "支出", showsEntity: false),
        ]
    )

    /// 無い: 未選択・消したアプリ・記録先の無いアプリ
    func testNoButtons() {
        XCTAssertTrue(catalog.widgetButtons(appId: nil, limit: 4).isEmpty)
        XCTAssertTrue(catalog.widgetButtons(appId: "gone", limit: 4).isEmpty)
        XCTAssertTrue(ControlCatalog(apps: [.init(id: "a3", name: "メモ")], recordTargets: [])
            .widgetButtons(appId: "a3", limit: 4).isEmpty)
        XCTAssertTrue(catalog.widgetButtons(appId: "a1", limit: 0).isEmpty)
    }

    /// 1 つ: 選んだアプリの記録先だけ(他のアプリは混ぜない)
    func testSingleButton() {
        let buttons = catalog.widgetButtons(appId: "a2", limit: 4)
        XCTAssertEqual(buttons.map(\.id), ["a2/expense"])
        XCTAssertEqual(buttons.first?.entityName, "支出")
    }

    /// 上限: 入らない分は切る。大きさごとの数
    func testLimitPerFamily() {
        XCTAssertEqual(catalog.widgetButtons(appId: "a1", limit: 4).map(\.entityId),
                       ["check", "water", "walk", "book"])
        XCTAssertEqual(WidgetActionLayout.limit(for: .systemMedium), 4)
        XCTAssertEqual(WidgetActionLayout.limit(for: .systemSmall), 2)
        XCTAssertEqual(WidgetActionLayout.limit(for: .accessoryRectangular), 1)
        for family in [WidgetFamily.systemSmall, .systemMedium, .accessoryRectangular] {
            let limit = WidgetActionLayout.limit(for: family)
            XCTAssertEqual(catalog.widgetButtons(appId: "a1", limit: limit).count, limit)
        }
    }

    /// 描いても落ちない・はみ出さない(無い・1 つ・上限)
    func testRendersWithoutOverflow() throws {
        let size = CGSize(width: 158, height: 158) // systemSmall
        let padding: CGFloat = 16
        let cases: [[ControlCatalog.RecordTarget]] = [
            [],
            catalog.widgetButtons(appId: "a2", limit: 2),
            catalog.widgetButtons(appId: "a1", limit: 2),
        ]
        for buttons in cases {
            let entry = ActionEntry(date: .now, appName: buttons.first?.appName, buttons: buttons,
                                    stat: .init(key: "k", label: "今日の回数", value: "3 件"))
            let view = ActionButtonsView(entry: entry)
                .padding(padding)
                .frame(width: size.width, height: size.height)
            let image = try XCTUnwrap(ImageRenderer(content: view).uiImage)
            XCTAssertEqual(image.size, size)

            let natural = ActionButtonsView(entry: entry)
                .frame(width: size.width - padding * 2)
                .fixedSize(horizontal: false, vertical: true)
            let height = ImageRenderer(content: natural).uiImage?.size.height ?? .infinity
            XCTAssertLessThanOrEqual(height, size.height - padding * 2, "\(buttons.count) 個")
        }
    }
}
