import XCTest

/// #1373: Mac でマウスを乗せても説明(ヘルプのタグ)が出ないボタン・クリックできる行を見つける。
/// ソースを読み、次のものの近く(前 3 行・後 13 行)にヘルプの付け方(`.help(`・`.macHelp(`・`iconButtonLabel(`・
/// `macIconButtonHelp(`)が無ければ落とす:
/// - `.onTapGesture`(クリックできる行・カード・点)
/// - ラベルがアイコンだけのボタン(`label: { Image(systemName: … }`・`Button { Image(systemName: … }`)
/// 押せる所でないもの(背景のクリックでキーボードを閉じる等)は `allowed` に理由と一緒に書く。
final class MacHelpCoverageTests: XCTestCase {
    /// 「ファイル名:そのタップの行の一部」→ ヘルプが要らない理由
    private let allowed: [String: String] = [
        "GridScreenLayout.swift:KeyboardState.dismiss()": "背景のクリックでキーボードを閉じるだけ(押せる所ではない)",
        "ComposeSheet.swift:session.skipReveal()": "組み上げの演出をクリックで飛ばすだけ(画面全体)",
        "MacScreenMapView.swift:select(nil)": "何もない所のクリックで選択を外すだけ",
    ]

    private static let marks = [".help(", ".macHelp(", "iconButtonLabel(", "macIconButtonHelp("]

    private var sourceRoots: [URL] {
        let tests = URL(fileURLWithPath: #filePath).deletingLastPathComponent() // Tests/MetamonRendererTests
        let package = tests.deletingLastPathComponent().deletingLastPathComponent() // Packages/MetamonRenderer
        let ios = package.deletingLastPathComponent().deletingLastPathComponent()
        return [package.appendingPathComponent("Sources/MetamonRenderer"), ios.appendingPathComponent("MetamonMac")]
    }

    func testTappableThingsHaveHelpOnMac() throws {
        var missing: [String] = []
        var scanned = 0
        for root in sourceRoots {
            guard FileManager.default.fileExists(atPath: root.path) else { continue }
            let files = FileManager.default.enumerator(at: root, includingPropertiesForKeys: nil)?
                .compactMap { $0 as? URL }.filter { $0.pathExtension == "swift" } ?? []
            for file in files {
                let lines = try String(contentsOf: file, encoding: .utf8).components(separatedBy: "\n")
                scanned += 1
                missing += Self.missingHelp(lines: lines, file: file.lastPathComponent).filter { hit in
                    !allowed.keys.contains { key in
                        let parts = key.split(separator: ":", maxSplits: 1).map(String.init)
                        return hit.hasPrefix(parts[0] + ":") && hit.contains(parts[1])
                    }
                }
            }
        }
        XCTAssertGreaterThan(scanned, 50, "ソースが読めていない(\(sourceRoots.map(\.path)))")
        XCTAssertEqual(missing, [], "ヘルプのタグ(.help / .macHelp / iconButtonLabel)が無い:\n" + missing.joined(separator: "\n"))
    }

    /// 見つけ方そのもののテスト
    func testDetectsMissingHelp() {
        let lines = [
            "Button { go() } label: { Image(systemName: \"plus\") }",
            "    .buttonStyle(.plain)",
            "Row().onTapGesture { open() }",
        ]
        XCTAssertEqual(Self.missingHelp(lines: lines, file: "A.swift").count, 2)
        let ok = ["Row()", "    .onTapGesture { open() }", "    .macHelp(L(\"クリックで開きます\"))"]
        XCTAssertEqual(Self.missingHelp(lines: ok, file: "B.swift"), [])
        let comment = ["// .onTapGesture はここでは使わない"]
        XCTAssertEqual(Self.missingHelp(lines: comment, file: "C.swift"), [])
    }

    static func missingHelp(lines: [String], file: String) -> [String] {
        var out: [String] = []
        for (i, line) in lines.enumerated() {
            let s = line.trimmingCharacters(in: .whitespaces)
            if s.hasPrefix("//") { continue }
            guard line.contains(".onTapGesture") || isIconOnlyButton(lines, at: i) else { continue }
            let window = lines[max(0, i - 3)..<min(lines.count, i + 14)].joined(separator: "\n")
            if !marks.contains(where: window.contains) {
                out.append("\(file):\(i + 1): \(s.prefix(100))")
            }
        }
        return out
    }

    private static func isIconOnlyButton(_ lines: [String], at i: Int) -> Bool {
        let line = lines[i]
        if line.range(of: #"(label:\s*\{|Button\s*(\(action:[^)]*\))?\s*\{)\s*Image\(systemName:"#, options: .regularExpression) != nil {
            return true
        }
        if line.range(of: #"label:\s*\{\s*$"#, options: .regularExpression) != nil, i + 2 < lines.count {
            return lines[i + 1].trimmingCharacters(in: .whitespaces).hasPrefix("Image(systemName:")
                && lines[i + 2].trimmingCharacters(in: .whitespaces).hasPrefix("}")
        }
        return false
    }
}
