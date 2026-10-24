import XCTest
@testable import MetamonDSL

/// `docs/dsl/examples/gallery-structure.json`(構造・ナビ・ネイティブ系の部品見本)が Swift 側でもフルにデコードできることを保証する。
final class GalleryStructureTests: XCTestCase {
    private func gallery() throws -> AppDSL { try ExamplesFixture.load("gallery-structure") }

    func testEveryStructureTypeDecodesAsKnown() throws {
        let dsl = try gallery()
        let types = Set(dsl.screens.flatMap(\.allComponents).map(\.type))
        for want: ComponentType in [.card, .disclosure, .carousel, .grid,
                                    .searchBar, .calendar, .shareButton, .openUrlButton,
                                    .mapView, .locationButton, .qrScanner, .timerDisplay] {
            XCTAssertTrue(types.contains(want), "gallery-structure に \(want) が無い")
        }
        XCTAssertFalse(types.contains(.unknown))
        XCTAssertTrue(ComponentType.card.isContainer)
        XCTAssertTrue(ComponentType.grid.isContainer)
        XCTAssertFalse(ComponentType.calendar.isContainer)
    }

    func testContainerAndLeafPropsDecode() throws {
        let dsl = try gallery()
        let containers = try XCTUnwrap(dsl.screen("containers"))
        let grid = try XCTUnwrap(containers.components.first { $0.type == .grid })
        XCTAssertEqual(grid.columns, 3)
        XCTAssertEqual(grid.children.count, 3)
        let carousel = try XCTUnwrap(containers.components.first { $0.type == .carousel })
        XCTAssertEqual(carousel.itemWidth, "medium")
        let disclosure = try XCTUnwrap(containers.components.first { $0.type == .disclosure })
        XCTAssertEqual(disclosure.expanded, false)
        XCTAssertEqual(disclosure.children.last?.type, .openUrlButton)
        XCTAssertEqual(disclosure.children.last?.url, "https://jphacks.com")

        let native = try XCTUnwrap(dsl.screen("native"))
        let list = try XCTUnwrap(native.components.first { $0.type == .list })
        XCTAssertEqual(list.searchField, "search")
        let cal = try XCTUnwrap(native.components.first { $0.type == .calendar })
        XCTAssertEqual(cal.dateField, "date")
        XCTAssertEqual(cal.onDateTap?.type, .navigate)
        XCTAssertEqual(cal.onDateTap?.presentation, .sheet)
        let map = try XCTUnwrap(native.components.first { $0.type == .mapView })
        XCTAssertEqual(map.locationField, "place")
        XCTAssertEqual(map.zoom, "city")
        XCTAssertEqual(native.referencedEntities, ["memo"])

        let form = try XCTUnwrap(dsl.screen("memo_form"))
        let timer = try XCTUnwrap(form.components.first { $0.type == .timerDisplay })
        XCTAssertEqual(timer.mode, "countdown")
        XCTAssertEqual(timer.seconds, 60)
        XCTAssertEqual(timer.field, "elapsed")
        XCTAssertEqual(form.components.first { $0.type == .shareButton }?.text, "メモ: {{note}}")
    }

    func testGridPlacementHasNoOverlap() throws {
        let dsl = try gallery()
        for s in dsl.screens {
            XCTAssertTrue(s.overlappingAreas.isEmpty, "\(s.id): area が重なっている")
            for c in s.components where c.area != nil {
                XCTAssertNotNil(c.gridArea, "\(s.id)/\(c.id): area \(c.area!) が解析できない")
            }
        }
    }
}
