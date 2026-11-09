import XCTest
import MetamonDSL
@testable import MetamonServices

/// 固定文言が英語で出る(String Catalog に en がある)。抜けると日本語のキーのまま出る。
final class EnglishCopyTests: XCTestCase {
    private var saved: String?
    override func setUp() { saved = setAppLanguage("en") }
    override func tearDown() { setAppLanguage(saved) }

    func testServiceCopyIsEnglish() {
        XCTAssertEqual(AuthError.alreadyLinked.errorDescription, "You're already signed in with Apple")
        XCTAssertEqual(RecordSyncEngine.Status.offline(pending: 2).label, "Offline, 2 unsent")
        XCTAssertEqual(TranscriberFailure.silent(seconds: 1.3, peakDecibels: -60).errorDescription,
                       "The audio was too quiet (recorded 1.3s, level -60dB). Speak toward the microphone")
        XCTAssertEqual(TranscriberFailure.tooShort(seconds: 0.4).errorDescription,
                       "The recording was too short (0.4s). Tap, speak, then tap again when you're done")
    }

    func testJapaneseStaysJapanese() {
        setAppLanguage("ja")
        XCTAssertEqual(RecordSyncEngine.Status.pending(3).label, "未送信 3 件")
        XCTAssertEqual(TranscriberFailure.tooShort(seconds: 0.4).errorDescription,
                       "録音が短すぎました(0.4秒)。タップしてから話し、話し終えてからもう一度タップしてください")
    }

    /// ソースの `L("…")` のキーがすべてカタログにあり、英訳が付いている(DSL・Services の両方)。
    func testUsedKeysExistInCatalog() throws {
        let services = URL(fileURLWithPath: #filePath).deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()
        try CatalogCoverage.assertCovered(sources: services.appendingPathComponent("Sources/MetamonServices"))
        try CatalogCoverage.assertCovered(sources: services.deletingLastPathComponent().appendingPathComponent("MetamonDSL/Sources/MetamonDSL"))
    }
}

/// アプリ内の UI 言語(`PackageLocalization` が読む設定)を切り替え、前の値を返す。nil は端末の言語。
@discardableResult
func setAppLanguage(_ code: String?) -> String? {
    let old = UserDefaults.standard.string(forKey: PackageLocalization.storageKey)
    UserDefaults.standard.set(code, forKey: PackageLocalization.storageKey)
    return old
}

/// `L("…")` のキー(補間は書式指定子として照合)とカタログ(`Resources/Localizable.xcstrings`)を突き合わせる。
enum CatalogCoverage {
    static func assertCovered(sources dir: URL, file: StaticString = #filePath, line: UInt = #line) throws {
        let catalogURL = dir.appendingPathComponent("Resources/Localizable.xcstrings")
        let catalog = try JSONSerialization.jsonObject(with: Data(contentsOf: catalogURL)) as? [String: Any]
        let strings = try XCTUnwrap(catalog?["strings"] as? [String: Any], file: file, line: line)
        let translated = strings.compactMap { key, value -> String? in
            let en = ((value as? [String: Any])?["localizations"] as? [String: Any])?["en"] as? [String: Any]
            return (en?["stringUnit"] as? [String: Any])?["value"] is String ? key : nil
        }
        let call = try NSRegularExpression(pattern: #"\bL\("((?:[^"\\]|\\\((?:[^()"]|"[^"]*")*\))*)"\)"#)
        let interp = try NSRegularExpression(pattern: #"\\\((?:[^()"]|"[^"]*")*\)"#)
        var used = 0
        let files = FileManager.default.enumerator(at: dir, includingPropertiesForKeys: nil)?.compactMap { $0 as? URL } ?? []
        for url in files where url.pathExtension == "swift" {
            let text = try String(contentsOf: url, encoding: .utf8)
            for m in call.matches(in: text, range: NSRange(text.startIndex..., in: text)) {
                let raw = String(text[Range(m.range(at: 1), in: text)!])
                // 補間は「%… の書式指定子」としてカタログのキーに当てる
                var pattern = "^", last = raw.startIndex
                for i in interp.matches(in: raw, range: NSRange(raw.startIndex..., in: raw)) {
                    let r = Range(i.range, in: raw)!
                    pattern += NSRegularExpression.escapedPattern(for: String(raw[last..<r.lowerBound])) + "%[0-9.$]*(?:@|lld|f)"
                    last = r.upperBound
                }
                let re = try NSRegularExpression(pattern: pattern + NSRegularExpression.escapedPattern(for: String(raw[last...])) + "$")
                let hit = translated.contains { re.firstMatch(in: $0, range: NSRange($0.startIndex..., in: $0)) != nil }
                XCTAssertTrue(hit, "カタログに英訳が無い: \(raw) (\(url.lastPathComponent))", file: file, line: line)
                used += 1
            }
        }
        XCTAssertGreaterThan(used, 0, "L(\"…\") が見つからない(パスの解決が壊れている): \(dir.path)", file: file, line: line)
    }
}
