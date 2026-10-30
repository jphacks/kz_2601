import XCTest
import MetamonDSL
import MetamonServices
@testable import Metamon

/// #415 頼み方のコツ: カテゴリ・審査用の版での絞り込み・英訳・ai-eval のケースとの一致。
@MainActor
final class PromptTipsTests: XCTestCase {
    private var allTips: [PromptTips.Tip] { PromptTips.all.flatMap(\.tips) }

    func testCategoriesCoverIssueTopics() {
        let ids = PromptTips.all.map(\.id)
        XCTAssertEqual(ids, ["tabs", "nav", "detail", "add", "stats", "notify", "share", "map", "photo", "fix"])
        for category in PromptTips.all {
            XCTAssertTrue((2...4).contains(category.tips.count), "\(category.id): 例は 2〜4 個")
        }
        XCTAssertEqual(Set(allTips.map(\.id)).count, allTips.count, "id の重複")
        XCTAssertEqual(Set(allTips.map(\.key)).count, allTips.count, "文の重複")
    }

    func testFullBuildShowsEverything() {
        XCTAssertEqual(PromptTips.Availability.from(allowedComponents: nil, disabledFeatures: nil, sharing: true), .full)
        XCTAssertEqual(PromptTips.categories(for: .full), PromptTips.all)
        #if !APP_STORE_REVIEW
        XCTAssertEqual(PromptTips.Availability.current, .full, "通常の版は全部出す")
        #endif
    }

    func testReviewBuildHidesStrippedFeatures() {
        let review = PromptTips.Availability.review
        XCTAssertTrue(review.features.isEmpty, "審査用の版は共有・撮影・nearby・health・道のりが無い: \(review.features)")
        let shown = PromptTips.categories(for: review)
        XCTAssertFalse(shown.map(\.id).contains("share"), "共有のカテゴリごと出さない(wording.md §3)")
        let ids = Set(shown.flatMap(\.tips).map(\.id))
        for hidden in ["stats_steps", "share_family", "share_public", "share_nearby", "map_route", "photo_camera"] {
            XCTAssertFalse(ids.contains(hidden), hidden)
        }
        // 残る機能(写真ライブラリ・現在地 1 回・地図・QR)の例は出す
        for kept in ["photo_attach", "photo_qr", "map_pins", "map_here", "tab_add", "notify_daily", "fix_undo"] {
            XCTAssertTrue(ids.contains(kept), kept)
        }
        for category in shown {
            XCTAssertGreaterThanOrEqual(category.tips.count, 2, "\(category.id): 審査用の版でも 2 個以上")
        }
    }

    /// 1 機能だけ外れたときも、その機能の例だけが消える。
    func testEachFeatureHidesOnlyItsTips() {
        for feature in PromptTips.Feature.allCases {
            var availability = PromptTips.Availability.full
            availability.features.remove(feature)
            let ids = Set(PromptTips.categories(for: availability).flatMap(\.tips).map(\.id))
            let expected = Set(allTips.filter { !$0.requires.contains(feature) }.map(\.id))
            XCTAssertEqual(ids, expected, "\(feature)")
        }
    }

    /// 審査員の目に入る文言(docs/store/wording.md): 「アプリを作る」系を使わない。
    func testWordingAvoidsAppMaking() {
        let texts = [PromptTips.titleKey, PromptTips.introKey] + PromptTips.all.map(\.titleKey) + allTips.map(\.key)
        for text in texts {
            XCTAssertFalse(text.contains("アプリ"), text)
        }
        for text in texts.map(english) {
            let lower = text.lowercased()
            XCTAssertFalse(lower.contains(" app") || lower.hasPrefix("app"), text)
        }
    }

    func testEnglishTranslationsExist() {
        let keys = [PromptTips.titleKey, PromptTips.introKey] + PromptTips.all.map(\.titleKey) + allTips.map(\.key)
        for key in keys {
            XCTAssertNotEqual(english(key), key, "英訳が無い: \(key)")
        }
    }

    /// 端末で処理する例(「元に戻して」)は ja / en とも AI に送らず undo になる。
    func testOnDeviceTipsAreUndo() {
        let onDevice = allTips.filter(\.onDevice)
        XCTAssertFalse(onDevice.isEmpty)
        for tip in onDevice {
            for text in [tip.key, english(tip.key)] {
                guard case .undo = VoiceIntent.detect(text) else { return XCTFail("undo にならない: \(text)") }
            }
        }
    }

    /// iOS の例文 = ai-eval の transcript(ja: tip_<id> / en: tip_<id>_en)。どちらかだけ直すと落ちる。
    func testEvalCasesMatchTips() throws {
        let url = URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()
            .appendingPathComponent("backend/internal/ai/eval/tips_cases.json")
        struct EvalCase: Decodable { var id: String; var locale: String?; var transcript: String }
        let cases = try JSONDecoder().decode([EvalCase].self, from: Data(contentsOf: url))
        let byID = Dictionary(uniqueKeysWithValues: cases.map { ($0.id, $0) })
        var expected = Set<String>()
        for tip in allTips where !tip.onDevice {
            let ja = "tip_\(tip.id)", en = "tip_\(tip.id)_en"
            expected.formUnion([ja, en])
            XCTAssertEqual(byID[ja]?.transcript, tip.key, ja)
            XCTAssertEqual(byID[ja]?.locale, "ja", ja)
            XCTAssertEqual(byID[en]?.transcript, english(tip.key), en)
            XCTAssertEqual(byID[en]?.locale, "en", en)
        }
        XCTAssertEqual(Set(byID.keys), expected, "ai-eval にだけ / iOS にだけある例")
    }

    private func english(_ key: String) -> String {
        guard let path = Bundle(for: AppRuntime.self).path(forResource: "en", ofType: "lproj"),
              let bundle = Bundle(path: path) else { return key }
        return bundle.localizedString(forKey: key, value: key, table: nil)
    }
}
