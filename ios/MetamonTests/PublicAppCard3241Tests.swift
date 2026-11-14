#if !APP_STORE_REVIEW
import XCTest
import MetamonDSL
@testable import Metamon

/// #3241: 「探す」カードの部品アイコン(`ComponentIcon.symbol`)と並び順(`DiscoverSort`)
final class PublicAppCard3241Tests: XCTestCase {
    func testEveryComponentTypeHasNonEmptySymbol() {
        for type in ComponentType.allCases {
            XCTAssertFalse(ComponentIcon.symbol(for: type.rawValue).isEmpty, "\(type.rawValue) の記号が空")
        }
    }

    func testKnownTypesMapToSpecificSymbols() {
        XCTAssertEqual(ComponentIcon.symbol(for: "stat"), "number")
        XCTAssertEqual(ComponentIcon.symbol(for: "chart"), "chart.bar.fill")
        XCTAssertEqual(ComponentIcon.symbol(for: "list"), ComponentIcon.symbol(for: "table"))
        XCTAssertEqual(ComponentIcon.symbol(for: "button"), ComponentIcon.symbol(for: "shareButton"))
    }

    func testUnknownTypeFallsBackToDefaultSymbol() {
        let fallback = "square.grid.2x2"
        XCTAssertEqual(ComponentIcon.symbol(for: "somethingNew"), fallback)
        XCTAssertEqual(ComponentIcon.symbol(for: ""), fallback)
    }

    func testDiscoverSortRawValuesAreServerNames() {
        XCTAssertEqual(DiscoverSort.allCases.map(\.rawValue), ["popular", "new"])
        XCTAssertEqual(DiscoverSort(rawValue: "popular"), .popular)
        XCTAssertEqual(DiscoverSort(rawValue: "new"), .new)
        XCTAssertNil(DiscoverSort(rawValue: "oldest"))
        XCTAssertEqual(DiscoverSort.popular.id, "popular")
    }
}
#endif
