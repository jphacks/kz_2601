import XCTest

/// L2: シートの「閉じる」は右上の文字のボタンにそろえる。個別に置くとまた位置と形がずれるので、ソースで守る。
final class SheetCloseConsistencyTests: XCTestCase {
    private func swiftSources() throws -> [(name: String, text: String)] {
        var ios = URL(fileURLWithPath: #filePath)
        for _ in 0..<5 { ios.deleteLastPathComponent() }
        let roots = ["Metamon", "Packages/MetamonRenderer/Sources"].map { ios.appending(path: $0) }
        return try roots.flatMap { root -> [(name: String, text: String)] in
            let files = FileManager.default.enumerator(at: root, includingPropertiesForKeys: nil)?.compactMap { $0 as? URL } ?? []
            return try files.filter { $0.pathExtension == "swift" }.map { ($0.lastPathComponent, try String(contentsOf: $0, encoding: .utf8)) }
        }
    }

    func testToolbarsUseSharedCloseItem() throws {
        let sources = try swiftSources()
        XCTAssertGreaterThan(sources.count, 100, "ソースを読めていない")
        for (name, text) in sources {
            XCTAssertNil(text.range(of: #"ToolbarItem\([^\n]*(\n[^\n]*)?"閉じる""#, options: .regularExpression),
                         "\(name): ツールバーの閉じるは SheetCloseToolbarItem を使う")
        }
    }

    func testSheetHeadersHaveNoCrossIcon() throws {
        for (name, text) in try swiftSources() where name.hasSuffix("Sheet.swift") {
            XCTAssertFalse(text.contains(#"Image(systemName: "xmark")"#), "\(name): シートの閉じるは SheetCloseButton を使う")
        }
    }
}
