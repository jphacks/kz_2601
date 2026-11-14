import XCTest
import MetamonDSL
import MetamonServices
import MetamonRenderer
@testable import Metamon

/// 動かない時は話しかけて直せる、の案内と「直してと頼む」(テスターの声)。
@MainActor
final class FixRequestTests: XCTestCase {
    private var saved: AppLanguage = .system

    override func setUp() {
        saved = LanguagePreference.shared.setting
        LanguagePreference.shared.setting = .ja
    }

    override func tearDown() { LanguagePreference.shared.setting = saved }

    func testDraftMentionsScreenAndPart() {
        XCTAssertEqual(FixRequest.draft(screenTitle: "家計簿", part: "保存"), "「家計簿」の画面の「保存」が動かないから直して")
        XCTAssertEqual(FixRequest.draft(screenTitle: "家計簿", part: nil), "「家計簿」の画面がうまく動かないから直して")
        XCTAssertEqual(FixRequest.draft(screenTitle: " ", part: "リンク"), "「リンク」が動かないから直して", "空の画面名は使わない")
        XCTAssertEqual(FixRequest.draft(screenTitle: nil, part: nil), "うまく動かない所があるから直して")
        XCTAssertEqual(FixRequest.draft(screenTitle: "ホーム", part: nil, problem: "未対応のアクション: foo"),
                       "「ホーム」の画面がうまく動かないから直して(「未対応のアクション: foo」と出ます)")
    }

    func testHintIsTheSameSentenceAsPromptTips() {
        XCTAssertEqual(FixRequest.hint, PromptTips.fixRequest)
        XCTAssertTrue(PromptTips.fixRequestKey.contains("動かないから直して"))
        XCTAssertFalse(PromptTips.fixRequestKey.contains("アプリ"), "審査の文言(wording.md)")
    }

    func testEnglishTranslationsExist() {
        guard let path = Bundle(for: AppRuntime.self).path(forResource: "en", ofType: "lproj"),
              let bundle = Bundle(path: path) else { return XCTFail("en.lproj が無い") }
        for key in [PromptTips.fixRequestKey, "直してと頼む", "「%@」の画面の「%@」が動かないから直して", "(「%@」と出ます)"] {
            XCTAssertNotEqual(bundle.localizedString(forKey: key, value: key, table: nil), key, key)
        }
        LanguagePreference.shared.setting = .en
        XCTAssertEqual(FixRequest.draft(screenTitle: "Budget", part: "Save"), "The \"Save\" on the \"Budget\" screen isn't working, fix it",
                       "英語は部品 → 画面の順(位置指定)")
    }

    /// 失敗のトーストに付き、押すと依頼文が入力欄に入って音声シートが開く(送らない)。別のトーストに替わったら外れる。
    func testFixOfferFollowsToastAndOpensComposer() throws {
        let rt = AppRuntime(document: try DSLExamples.document("kakeibo", version: 1), services: BuildConfig.make(environment: .mock))
        let screen = try XCTUnwrap(rt.dsl.screens.first)
        rt.reportFixable("未対応のアクション: foo", ctx: ActionContext(route: Route(screen: screen.id)))
        XCTAssertEqual(rt.toast, "未対応のアクション: foo")
        let offer = try XCTUnwrap(rt.fixOffer)
        XCTAssertTrue(offer.draft.contains("「\(screen.title)」の画面"), offer.draft)

        rt.requestFix()
        XCTAssertNil(rt.fixOffer)
        XCTAssertNil(rt.toast)
        XCTAssertTrue(rt.showVoiceSheet)
        XCTAssertEqual(rt.composerDraft, offer.draft)

        rt.reportFixable("x", ctx: nil)
        XCTAssertNotNil(rt.fixOffer)
        rt.toast = "保存しました"
        XCTAssertNil(rt.fixOffer, "別のトーストには付けない")
    }

    func testNoFixOfferOnVoiceOnlyScreen() throws {
        let rt = AppRuntime(document: try DSLExamples.document(DSLExamples.initial, version: 0), services: BuildConfig.make(environment: .mock))
        XCTAssertTrue(rt.dsl.isVoiceOnly)
        rt.reportFixable("x", ctx: nil)
        XCTAssertEqual(rt.toast, "x")
        XCTAssertNil(rt.fixOffer, "まだ何も作っていない時は出さない")
    }

    /// 直し方の例の順番は変えない(同じカードに 1 行足しただけ)。
    func testEditHintKeepsItsTurn() {
        var pending = Set(PostBuildNotice.allCases)
        var shown: [PostBuildNotice] = []
        while let next = PostBuildNotice.current(pending) { shown.append(next); pending.remove(next) }
        XCTAssertEqual(shown, [.paywall, .summary, .publicData, .retention, .editHint, .accountLink])
    }
}
