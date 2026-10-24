import XCTest
@testable import MetamonDSL

final class GridAreaTests: XCTestCase {
    func testParsing() {
        XCTAssertEqual(GridArea("A1:C3").map { [$0.col1, $0.row1, $0.col2, $0.row2] }, [0, 1, 2, 3])
        XCTAssertEqual(GridArea("A1:C3")?.colSpan, 3)
        XCTAssertEqual(GridArea("A1:C3")?.rowSpan, 3)
        XCTAssertNil(GridArea("G1:G2"))
        XCTAssertNil(GridArea("A0:B1"))
        XCTAssertNil(GridArea("C3:A1"))
        XCTAssertNil(GridArea("A1"))
    }

    func testOverlap() {
        XCTAssertTrue(GridArea("A1:C3")!.overlaps(GridArea("C3:F6")!))
        XCTAssertFalse(GridArea("A1:C3")!.overlaps(GridArea("D1:F3")!))
        XCTAssertFalse(GridArea("A1:F3")!.overlaps(GridArea("A4:F6")!))
    }

    /// 正典サンプルの area は全て有効で、兄弟同士で重ならない(backend の検証と同じ約束)。
    func testExampleAreasAreValidAndNonOverlapping() throws {
        for name in DSLExamples.names {
            let dsl = try ExamplesFixture.load(name)
            for s in dsl.screens {
                for c in s.components where c.area != nil {
                    XCTAssertNotNil(c.gridArea, "\(name)/\(s.id): bad area \(c.area!)")
                }
                let overlaps = s.overlappingAreas
                XCTAssertTrue(overlaps.isEmpty, "\(name)/\(s.id): \(overlaps.map { "\($0.0.id)×\($0.1.id)" })")
                XCTAssertFalse(s.gridPlacement.placed.isEmpty, "\(name)/\(s.id): examples should use the grid")
                for c in s.components {
                    guard let local = c.localGridPlacement else { continue }
                    for i in local.indices {
                        for j in local.indices where j > i {
                            XCTAssertFalse(local[i].area.overlaps(local[j].area), "\(name)/\(s.id)/\(c.id): children overlap")
                        }
                    }
                }
            }
        }
    }

    /// Container 内グリッド(#81): card / section / vstack の子が全員 area を持つ時だけ配置を返す。
    func testLocalGridPlacement() throws {
        let dsl = try ExamplesFixture.load("water")
        let home = try XCTUnwrap(dsl.screens.first { $0.id == "home" })
        let card = try XCTUnwrap(home.components.first { $0.type == .card })
        XCTAssertEqual(card.localGridPlacement?.map(\.area.col1), [0, 3])
        let section = try XCTUnwrap(home.components.first { $0.id == "quick" })
        XCTAssertEqual(section.localGridPlacement?.count, 2)
        // area の無い子だけの section は従来どおり積む
        XCTAssertNil(home.components.first { $0.id == "recent" }?.localGridPlacement)
    }
}
