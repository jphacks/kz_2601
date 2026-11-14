import XCTest
import MetamonRenderer
@testable import Metamon

/// 英語で数と名詞が合う(1 records・1 people にならない)。String Catalog の plural で 1 と 2 以上を分ける。
@MainActor
final class EnglishPluralsTests: XCTestCase {
    private var saved: AppLanguage = .system

    override func setUp() {
        saved = LanguagePreference.shared.setting
        LanguagePreference.shared.setting = .en
    }

    override func tearDown() { LanguagePreference.shared.setting = saved }

    func testPluralsMatchCount() {
        let cases: [(Int, String, String)] = [
            (1, L10n.tr("\("Ko") ・ \(1) 人が利用"), "Ko · 1 user"),
            (2, L10n.tr("\("Ko") ・ \(2) 人が利用"), "Ko · 2 users"),
            (1, L10n.tr("\("Ko")さんから「\("Log")」が \(1) 件届きました"), "Ko sent 1 “Log” record"),
            (2, L10n.tr("\("Ko")さんから「\("Log")」が \(2) 件届きました"), "Ko sent 2 “Log” records"),
            (1, L10n.tr("\("Ko")さんから「\("Log")」を \(1) 件追加しました"), "Added 1 “Log” record from Ko"),
            (2, L10n.tr("\("Ko")さんから「\("Log")」を \(2) 件追加しました"), "Added 2 “Log” records from Ko"),
            (1, L10n.tr("「\("Log")」\(1) 件"), "1 “Log” record"),
            (2, L10n.tr("「\("Log")」\(2) 件"), "2 “Log” records"),
            (1, L10n.tr("\(1) 人"), "1 person"), (2, L10n.tr("\(2) 人"), "2 people"),
            (1, L10n.tr("\(1) 件の記録を送れませんでした。送らずにログアウトすると、この記録は消えます。"),
             "1 record couldn’t be sent. If you sign out without sending, it’ll be lost."),
            (2, L10n.tr("\(2) 件の記録を送れませんでした。送らずにログアウトすると、この記録は消えます。"),
             "2 records couldn’t be sent. If you sign out without sending, they’ll be lost."),
            (1, L10n.tr("まだ送っていない記録が \(1) 件あります。送ってからログアウトします。"),
             "You have 1 unsent record. It’ll be sent before you sign out."),
            (2, L10n.tr("まだ送っていない記録が \(2) 件あります。送ってからログアウトします。"),
             "You have 2 unsent records. They’ll be sent before you sign out."),
            (1, L10n.tr("\(1) 文字までです"), "Up to 1 character"), (2, L10n.tr("\(2) 文字までです"), "Up to 2 characters"),
            (1, ErrorCopy.busyBody(retryAfter: 1), "Please wait about 1 second and try again"),
            (2, ErrorCopy.busyBody(retryAfter: 2), "Please wait about 2 seconds and try again"),
            (1, L10n.tr("「探す」に公開中・\(1) 人がインストール"), "Published on Discover · 1 install"),
            (2, L10n.tr("「探す」に公開中・\(2) 人がインストール"), "Published on Discover · 2 installs"),
            (1, L10n.tr("\(1) 回続けてうまくいっていません。"), "Failed 1 time in a row."),
            (2, L10n.tr("\(2) 回続けてうまくいっていません。"), "Failed 2 times in a row."),
            (1, L10n.tr("保存した場所 \(1) か所の近くに着いたら知らせます"), "You'll be notified when you arrive near 1 saved place"),
            (2, L10n.tr("保存した場所 \(2) か所の近くに着いたら知らせます"), "You'll be notified when you arrive near 2 saved places"),
            (1, L10n.tr("無料プランでは新しいアプリを 1 日 \(1) 回まで作れます。作ったアプリの改変は回数に数えません。"),
             "The free plan lets you create 1 new app a day. Editing your apps doesn't count."),
            (2, L10n.tr("無料プランでは新しいアプリを 1 日 \(2) 回まで作れます。作ったアプリの改変は回数に数えません。"),
             "The free plan lets you create 2 new apps a day. Editing your apps doesn't count."),
        ]
        for (n, actual, expected) in cases { XCTAssertEqual(actual, expected, "n=\(n)") }
    }
}
