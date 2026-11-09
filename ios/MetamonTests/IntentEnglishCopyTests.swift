import XCTest
import AppIntents
@testable import Metamon

/// Siri / ショートカットの文言(App Intents)に英訳がある。返事はアプリ名・内容・項目名をデータのまま埋める。
final class IntentEnglishCopyTests: XCTestCase {
    private var en: Bundle!

    override func setUpWithError() throws {
        let path = try XCTUnwrap(Bundle(for: AppRuntime.self).path(forResource: "en", ofType: "lproj"), "en.lproj が無い")
        en = try XCTUnwrap(Bundle(path: path))
    }

    private func english(_ key: String) -> String {
        en.localizedString(forKey: key, value: "__missing__", table: nil)
    }

    private func assertTranslated(_ keys: [String], file: StaticString = #filePath, line: UInt = #line) {
        for key in keys {
            let value = english(key)
            XCTAssertNotEqual(value, "__missing__", "英語が無い: \(key)", file: file, line: line)
            XCTAssertNotEqual(value, key, "英語が日本語のまま: \(key)", file: file, line: line)
        }
    }

    func testAddedMessageKeysHaveEnglish() {
        let keys = [(true, true), (false, true), (true, false), (false, false)].map { noRefs, sent in
            AddRecordIntent.addedMessage(appName: "家計簿", what: "ランチ 800円", refs: noRefs ? [] : ["カテゴリ: 食費"], sent: sent).key
        }
        XCTAssertEqual(Set(keys).count, 4)
        assertTranslated(keys)
    }

    func testAddedMessageKeepsDataUntranslated() {
        let resource = AddRecordIntent.addedMessage(appName: "家計簿", what: "ランチ 800円", refs: ["カテゴリ: 食費"], sent: true)
        XCTAssertEqual(resource.key, "%@に「%@」(%@)を追加しました")
        let text = String(format: english(resource.key), "家計簿", "ランチ 800円", "カテゴリ: 食費")
        XCTAssertEqual(text, "Added “ランチ 800円” (カテゴリ: 食費) to 家計簿")
    }

    func testIntentCopyHasEnglish() {
        var keys = [
            OpenTailorAppIntent.title.key,
            AddRecordIntent.title.key,
            TalkToTailorItIntent.title.key,
            TailorAppEntity.typeDisplayRepresentation.name.key,
            RecordTargetEntity.typeDisplayRepresentation.name.key,
            "%@はいくつですか?",
            "内容", "例: ランチ 800円", "何を記録しますか?",
            "数値", "金額・回数など。省略すると内容の中の数字を使います",
        ]
        keys += [TailorIntentError.notReady, .targetNotFound("家計簿"), .missingFields(["金額"])].map(\.localizedStringResource.key)
        assertTranslated(keys)
    }

    /// Siri のフレーズ(AppShortcuts.xcstrings)。英語もアプリ名を含み、パラメータは日本語と同じ。
    func testShortcutPhrasesHaveEnglish() {
        let phrases = [
            "${applicationName}で${target}に記録",
            "${applicationName}で${target}に記録を追加",
            "${applicationName}で記録を追加",
            "${applicationName}に記録",
            "${applicationName}で${target}を開く",
            "${applicationName}で${target}を開いて",
            "${applicationName}で記録帳を開く",
            "${applicationName}でアプリを開く",
            "${applicationName}で話しかける",
            "${applicationName}に話しかける",
        ]
        for key in phrases {
            let value = en.localizedString(forKey: key, value: "__missing__", table: "AppShortcuts")
            XCTAssertNotEqual(value, "__missing__", "英語のフレーズが無い: \(key)")
            XCTAssertNotEqual(value, key, "英語のフレーズが日本語のまま: \(key)")
            XCTAssertTrue(value.contains("${applicationName}"), "英語のフレーズにアプリ名が無い: \(value)")
            XCTAssertEqual(value.contains("${target}"), key.contains("${target}"), "パラメータが合わない: \(value)")
        }
        let titles = ["記録を追加", "記録帳を開く", "アプリを開く", "話しかける"]
        assertTranslated(titles)
    }
}
