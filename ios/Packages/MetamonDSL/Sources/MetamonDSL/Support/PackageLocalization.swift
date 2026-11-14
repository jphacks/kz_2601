import Foundation

/// パッケージ内の固定文言を、アプリ内の UI 言語で引く(DSL・Services 共通)。
/// 設定値は Renderer の `LanguagePreference` と同じ UserDefaults キーを読む(DSL は Renderer に依存できないため)。
public enum PackageLocalization {
    public static let storageKey = "metamon.appLanguage"
    static let supported = ["ja", "en"]

    /// 実際に使う言語コード("ja" | "en")。`system`(未設定)は端末の優先言語に従う。
    public static var code: String {
        switch UserDefaults.standard.string(forKey: storageKey) {
        case "ja": "ja"
        case "en": "en"
        default: Bundle.preferredLocalizations(from: supported, forPreferences: Locale.preferredLanguages).first ?? "ja"
        }
    }

    /// `bundle` の String Catalog から現在の言語で引く。キーは日本語のソース文字列。
    public static func string(_ key: String.LocalizationValue, bundle: Bundle) -> String {
        let code = code
        let lproj = bundle.path(forResource: code, ofType: "lproj").flatMap(Bundle.init(path:)) ?? bundle
        let current = Locale.current
        let locale = current.language.languageCode?.identifier == code ? current : Locale(identifier: code == "ja" ? "ja_JP" : "en_US")
        return String(localized: key, bundle: lproj, locale: locale)
    }
}

/// DSL 内の固定文言(`Bundle.module` の String Catalog)。
func L(_ key: String.LocalizationValue) -> String {
    PackageLocalization.string(key, bundle: .module)
}
