import Foundation
import SwiftUI
import MetamonRenderer

/// 本体の `String` を返す文言(#44)。`Text("…")` リテラルはルートの `\.locale` で自動解決されるので不要。
/// キーは日本語のソース文字列(`Resources/Localizable.xcstrings`)。body 内で呼べば言語切替で再描画される。
enum L10n {
    static func tr(_ key: String.LocalizationValue) -> String {
        LanguagePreference.shared.string(key, bundle: .main)
    }

    /// 審査用の版(`APP_STORE_REVIEW`)だけ別の文言にする(#468)。「アプリを作る / 生成 / 変身」を避け、
    /// 「記録帳を用意する・直す」と言う(docs/store/wording.md)。通常の版は `normal` だけを評価する(今の文言のまま)。
    static func pick(_ normal: @autoclosure () -> String, review: @autoclosure () -> String) -> String {
        #if APP_STORE_REVIEW
        review()
        #else
        normal()
        #endif
    }

    /// `pick` の `Text` / `Label` 用。`LocalizedStringKey` のまま渡すので、通常の版は `Text("…")` リテラルと同じに解決される。
    static func key(_ normal: LocalizedStringKey, review: LocalizedStringKey) -> LocalizedStringKey {
        #if APP_STORE_REVIEW
        review
        #else
        normal
        #endif
    }
}
