// 審査用ビルドでは外す(β の文言を App Store の版に入れない・#313・docs/ios/review-build.md)。
#if !APP_STORE_REVIEW
import Foundation

/// β の案内の中身(#313)。キーは日本語(英語は `Localizable.xcstrings`)。iOS と Mac で共有(#971)。
enum BetaWelcomeContent {
    /// 試してほしい依頼の例。タップで入力欄に入る(送らない)
    static let tryKeys: [String] = [
        "家計簿アプリを作って",
        "毎日の体重を記録して、グラフで見せて",
        "筋トレの記録アプリを作って",
        "入力の画面に「メモ」の欄を足して",
    ]

    /// まだできないこと
    static let limitKeys: [String] = [
        "画面がたくさんつながるアプリや、ゲームのように動くアプリは、思った通りにできないことがあります。",
        "カレンダー・連絡先・銀行など、ほかのアプリやサービスのデータは読み込めません。",
        "AI が作るので、同じ頼み方でも毎回少し違うものができます。",
    ]
}
#endif
