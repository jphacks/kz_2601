import XCTest
import MetamonDSL
@testable import MetamonRenderer
import MetamonServices
@testable import Metamon

/// Mac の「直してと頼む」(#1885・iOS の FixRequestTests と同じ決まり)。
/// アプリの作りで直せる失敗(未対応のアクション・フォームの無い画面の保存・開けないリンク)のトーストにだけ付き、
/// 押すと依頼文を入れて会話を開く(送らない)。入力漏れには付けない。
@MainActor
final class MacFixRequestTests: XCTestCase {
    private let home = Route(screen: "home", stack: Route.tabStack("home"))
    private var savedLanguage: AppLanguage = .system

    override func setUp() {
        super.setUp()
        savedLanguage = LanguagePreference.shared.setting
        LanguagePreference.shared.setting = .ja
    }

    override func tearDown() {
        LanguagePreference.shared.setting = savedLanguage
        super.tearDown()
    }

    private func runtime(_ example: String = "kakeibo") throws -> MacAppRuntime {
        let url = try XCTUnwrap(Bundle(for: MacAppRuntime.self).url(forResource: example, withExtension: "json"))
        return MacAppRuntime(appId: "test-mac-\(UUID().uuidString.lowercased())", dsl: try AppDSL.load(contentsOf: url),
                             ai: MockAIClient(delay: .zero), isAIConsentGranted: { false })
    }

    private func action(_ json: String) throws -> Action {
        try JSONDecoder().decode(Action.self, from: Data(json.utf8))
    }

    @MainActor private final class Drafts { var values: [String] = [] }

    private func connected(_ rt: MacAppRuntime) -> Drafts {
        let drafts = Drafts()
        rt.onAskFix = { drafts.values.append($0) }
        return drafts
    }

    func testUnknownActionOffersFixAndOpensComposerWithDraft() async throws {
        let rt = try runtime()
        let drafts = connected(rt)
        await rt.run(try action(#"{"type":"teleport"}"#), ActionContext(route: home))
        let message = L10n.tr("このボタンは Mac ではまだ使えません")
        XCTAssertEqual(rt.toast, message)
        let offer = try XCTUnwrap(rt.fixOffer, "未対応のアクションには付ける")
        XCTAssertEqual(offer.message, message)
        XCTAssertEqual(offer.draft, "「家計簿」の画面がうまく動かないから直して(「\(message)」と出ます)")

        rt.requestFix()
        XCTAssertEqual(drafts.values, [offer.draft], "依頼文を入れて会話を開く(送らない)")
        XCTAssertNil(rt.toast)
        XCTAssertNil(rt.fixOffer)
        rt.requestFix()
        XCTAssertEqual(drafts.values.count, 1, "1 回だけ")
    }

    func testBadLinkOffersFixNamingTheLink() async throws {
        let rt = try runtime()
        _ = connected(rt)
        await rt.run(try action(#"{"type":"openUrl","url":"http://example.com"}"#), ActionContext(route: home))
        let offer = try XCTUnwrap(rt.fixOffer)
        XCTAssertTrue(offer.draft.hasPrefix("「家計簿」の画面の「リンク」が動かないから直して"), offer.draft)
    }

    func testSaveWithoutFormOffersFixButMissingInputDoesNot() async throws {
        let rt = try runtime()
        _ = connected(rt)
        await rt.run(try action(#"{"type":"saveRecord","entity":"expense"}"#), ActionContext(route: home))
        XCTAssertEqual(rt.toast, L10n.tr("保存するフォームがありません"))
        let offer = try XCTUnwrap(rt.fixOffer, "フォームの無い画面の保存ボタンはアプリの作りの問題")
        XCTAssertTrue(offer.draft.hasPrefix("「家計簿」の画面の「保存」が動かないから直して"), offer.draft)

        rt.toast = nil
        let form = FormDraft(entity: try XCTUnwrap(rt.dsl.entity("expense")))
        await rt.run(try action(#"{"type":"saveRecord","entity":"expense"}"#), ActionContext(route: home, form: form))
        XCTAssertNotNil(rt.toast, "入力漏れは知らせる")
        XCTAssertNil(rt.fixOffer, "入力漏れは利用者が直す(付けない)")
    }

    func testOfferFollowsOnlyItsToast() async throws {
        let rt = try runtime()
        _ = connected(rt)
        rt.reportFixable("x", ctx: nil)
        XCTAssertEqual(rt.fixOffer?.draft, "うまく動かない所があるから直して(「x」と出ます)")
        rt.toast = "保存しました"
        XCTAssertNil(rt.fixOffer, "別のトーストには付けない")
    }

    /// 同じ文言でも、`reportFixable` 以外のトーストには付けない
    func testPlainToastWithSameMessageDropsOffer() async throws {
        let rt = try runtime()
        _ = connected(rt)
        rt.reportFixable("保存するフォームがありません", ctx: nil)
        XCTAssertNotNil(rt.fixOffer)
        rt.toast = "保存するフォームがありません"
        XCTAssertNil(rt.fixOffer, "ふつうのトーストに替わったら外す")

        // 入力漏れ(フォームあり)の保存: 前のトーストと同じ文言でも付けない
        rt.reportFixable(L10n.tr("保存するフォームがありません"), ctx: nil)
        let form = FormDraft(entity: try XCTUnwrap(rt.dsl.entity("expense")))
        rt.reportInvalidSave(L10n.tr("保存するフォームがありません"), ActionContext(route: home, form: form))
        XCTAssertEqual(rt.toast, L10n.tr("保存するフォームがありません"))
        XCTAssertNil(rt.fixOffer, "入力漏れの分岐では外す")
    }

    func testNoOfferWithoutComposerOrOnVoiceOnlyApp() async throws {
        let rt = try runtime()
        rt.reportFixable("x", ctx: nil)
        XCTAssertEqual(rt.toast, "x")
        XCTAssertNil(rt.fixOffer, "会話につながっていない所では出さない")

        let initial = try runtime("initial")
        XCTAssertTrue(initial.dsl.isVoiceOnly)
        _ = connected(initial)
        initial.reportFixable("x", ctx: nil)
        XCTAssertNil(initial.fixOffer, "まだ何も作っていない時は出さない")
    }

    func testFixFromSheetClosesSheetFirst() async throws {
        let rt = try runtime()
        let drafts = connected(rt)
        await rt.run(try action(#"{"type":"navigate","screen":"expense_form","presentation":"sheet"}"#), ActionContext(route: home))
        let sheetRoute = try XCTUnwrap(rt.sheet)
        await rt.run(try action(#"{"type":"teleport"}"#), ActionContext(route: sheetRoute))
        XCTAssertNotNil(rt.fixOffer)
        rt.requestFix()
        XCTAssertNil(rt.sheet)
        XCTAssertTrue(drafts.values.isEmpty, "シートを閉じ終えるまで開かない(シートを重ねない)")
        rt.askFixAfterModal() // シートの onDismiss
        XCTAssertEqual(drafts.values.count, 1)
        rt.askFixAfterModal()
        XCTAssertEqual(drafts.values.count, 1, "1 回だけ開く")
    }

    /// シートの上に全画面: 1 枚ずつ閉じ、最後の 1 枚が閉じ終わってから会話を開く
    func testFixFromStackedModalsClosesOneAtATime() async throws {
        let rt = try runtime()
        let drafts = connected(rt)
        rt.sheet = Route(screen: "expense_form", stack: "sheet")
        rt.fullScreen = Route(screen: "expense_form", stack: "full")
        rt.reportFixable("x", ctx: nil)
        rt.requestFix()
        XCTAssertNil(rt.fullScreen, "上の 1 枚だけ閉じる")
        XCTAssertNotNil(rt.sheet, "同時には閉じない")
        rt.askFixAfterModal() // 全画面の onDismiss
        XCTAssertNil(rt.sheet, "次の 1 枚を閉じる")
        XCTAssertTrue(drafts.values.isEmpty, "最後の 1 枚が閉じ終わるまで開かない")
        rt.askFixAfterModal() // シートの onDismiss
        XCTAssertEqual(drafts.values, ["うまく動かない所があるから直して(「x」と出ます)"])
    }

    /// 直し方の例・頼み方のコツの一文。Mac は「話しかける」を ⌘K / ⇧⌘M の頼み方に言い換える
    func testMacHintMentionsShortcutsAndHasEnglish() throws {
        XCTAssertTrue(FixRequest.macHint.contains("動かないから直して"))
        XCTAssertTrue(FixRequest.macHint.contains("⌘K"))
        XCTAssertFalse(FixRequest.macHintKey.contains("アプリ"), "審査の文言(wording.md)")
        let path = try XCTUnwrap(Bundle(for: MacAppRuntime.self).path(forResource: "en", ofType: "lproj"))
        let en = try XCTUnwrap(Bundle(path: path))
        for key in [FixRequest.macHintKey, "直してと頼む", "直す依頼を入力欄に入れて、会話を開きます",
                    "「%@」の画面の「%@」が動かないから直して", "(「%@」と出ます)", "リンク"] {
            XCTAssertNotEqual(en.localizedString(forKey: key, value: key, table: nil), key, key)
        }
        LanguagePreference.shared.setting = .en
        XCTAssertEqual(FixRequest.draft(screenTitle: "Budget", part: "Save"), "The \"Save\" on the \"Budget\" screen isn't working, fix it")
    }
}
