import Foundation
import MetamonRenderer

/// 本体の `String` を返す文言(#44)。`Text("…")` リテラルはルートの `\.locale` で自動解決されるので不要。
/// キーは日本語のソース文字列(`Resources/Localizable.xcstrings`)。body 内で呼べば言語切替で再描画される。
enum L10n {
    static func tr(_ key: String.LocalizationValue) -> String {
        LanguagePreference.shared.string(key, bundle: .main)
    }
}
