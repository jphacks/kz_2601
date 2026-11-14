import XCTest
@testable import Metamon

/// 設定の「ライセンス」(#306)。同梱の一覧が読め、Package.resolved の依存が全部入っている。
/// 一覧が古い時の直し方: `python3 ios/scripts/third-party-licenses.py`
final class ThirdPartyLicensesTests: XCTestCase {
    func testBundledListLoads() throws {
        let entries = try XCTUnwrap(ThirdPartyLicenses.load(), "ThirdPartyLicenses.json が読めない")
        let first = try XCTUnwrap(entries.first)
        XCTAssertEqual(first.kind, .app, "先頭はこのアプリ自身の表示")
        XCTAssertEqual(first.license, "MIT")
        XCTAssertTrue(first.text.contains("Copyright (c) 2026 JPHACKS"))
        XCTAssertEqual(entries.filter { $0.kind == .app }.count, 1)
        for e in entries {
            XCTAssertFalse(e.license.isEmpty, e.id)
            XCTAssertFalse(e.text.isEmpty, e.id)
            XCTAssertFalse(ThirdPartyLicenses.paragraphs(e.text).isEmpty, e.id)
        }
    }

    func testEveryResolvedPackageIsListed() throws {
        let entries = try XCTUnwrap(ThirdPartyLicenses.load())
        let listed = Dictionary(uniqueKeysWithValues: entries.filter { $0.kind == .dependency }.map { ($0.id, $0) })
        let url = URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent().deletingLastPathComponent()
            .appendingPathComponent("Packages/MetamonServices/Package.resolved")
        struct Resolved: Decodable {
            struct Pin: Decodable {
                struct State: Decodable { let version: String? }
                let identity: String
                let state: State
            }
            let pins: [Pin]
        }
        let pins = try JSONDecoder().decode(Resolved.self, from: Data(contentsOf: url)).pins
        XCTAssertFalse(pins.isEmpty)
        for pin in pins {
            let entry = listed[pin.identity]
            XCTAssertNotNil(entry, "\(pin.identity) が一覧に無い")
            XCTAssertEqual(entry?.version, pin.state.version, "\(pin.identity) の版が古い")
        }
        XCTAssertEqual(listed.count, pins.count, "Package.resolved に無い依存が一覧にある")
    }

    func testParagraphsJoinWrappedLines() {
        XCTAssertEqual(ThirdPartyLicenses.paragraphs("  a\n  b\n\n\n c \n"), ["a b", "c"])
    }
}
