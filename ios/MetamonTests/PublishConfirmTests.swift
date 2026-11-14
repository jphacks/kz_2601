import XCTest
import MetamonRenderer
@testable import Metamon

/// 「みんなに公開」の前の確認(Android #1297 と同じ 4 つの注意)。
@MainActor
final class PublishConfirmTests: XCTestCase {
    private var saved: AppLanguage = .system

    override func setUp() { saved = LanguagePreference.shared.setting }
    override func tearDown() { LanguagePreference.shared.setting = saved }

    func testFourCautionsInJapanese() {
        LanguagePreference.shared.setting = .ja
        XCTAssertEqual(PublishConfirmCopy.title(appName: "家計簿"), "「家計簿」を公開しますか?")
        XCTAssertEqual(PublishConfirmCopy.points.map(\.text), [
            "「探す」から誰でも見つけて、自分のアプリとして入れられます",
            "あなただけの記録は含まれません",
            "名前・説明・タグに、住所や電話番号などの個人の情報を書かないでください",
            "このシェアの画面から、いつでも公開をやめられます",
        ])
    }

    func testCautionsAreTranslatedInEnglish() {
        LanguagePreference.shared.setting = .en
        XCTAssertEqual(PublishConfirmCopy.title(appName: "Budget"), "Publish “Budget”?")
        XCTAssertEqual(PublishConfirmCopy.points.count, 4)
        for p in PublishConfirmCopy.points {
            XCTAssertNil(p.text.range(of: "\\p{Han}|\\p{Hiragana}|\\p{Katakana}", options: .regularExpression), "訳が無い: \(p.text)")
        }
    }
}
