import XCTest
import MetamonDSL
@testable import MetamonRenderer

/// `bracket`(トーナメント表): どのビルドにも登録・勝ち数の field の条件・回戦の名前。
final class BracketLeafTests: XCTestCase {
    private func tournament() throws -> AppDSL {
        var url = URL(fileURLWithPath: #filePath)
        for _ in 0..<6 { url.deleteLastPathComponent() }
        return try AppDSL.load(contentsOf: url.appending(path: "docs/dsl/examples/tournament.json"))
    }

    @MainActor func testRegisteredInEveryBuild() throws {
        XCTAssertNotNil(ComponentRegistry.standard.builder(for: "bracket"))
        XCTAssertFalse(ClientCapabilities.reviewExcludedComponents.contains(.bracket), "OS の機能も共有も使わないので審査用ビルドにも入れる")
        XCTAssertTrue(ClientCapabilities.allComponents.contains("bracket"))
        XCTAssertTrue(ClientCapabilities.supportedFeatures.contains("component.bracket"))
        let c = try XCTUnwrap(tournament().screens.flatMap(\.allComponents).first { $0.type == .bracket })
        XCTAssertTrue(c.rendersOwnSurface)
    }

    func testWinsFieldMustBeStoredInteger() throws {
        let dsl = try tournament()
        var c = try XCTUnwrap(dsl.screens.flatMap(\.allComponents).first { $0.type == .bracket })
        XCTAssertEqual(BracketLeaf.winsField(c, in: dsl.entity("entrant")), "wins")
        c.winsField = "name"
        XCTAssertNil(BracketLeaf.winsField(c, in: dsl.entity("entrant")), "string の field には書かない(表示だけ)")
        c.winsField = "nope"
        XCTAssertNil(BracketLeaf.winsField(c, in: dsl.entity("entrant")))
    }

    func testRoundTitles() {
        XCTAssertEqual((1...4).map { BracketLeaf.roundTitle($0, of: 4) }, [L("\(1) 回戦"), L("準々決勝"), L("準決勝"), L("決勝")])
        XCTAssertEqual(BracketLeaf.roundTitle(1, of: 1), L("決勝"))
    }
}
