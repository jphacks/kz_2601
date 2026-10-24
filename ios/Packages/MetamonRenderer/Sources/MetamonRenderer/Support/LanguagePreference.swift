import Foundation
import Observation

/// UI 言語の設定値(#44)。`system` は端末の言語(ja / en のうち近い方)に従う。
public enum AppLanguage: String, CaseIterable, Sendable, Identifiable {
    case system, ja, en
    public var id: String { rawValue }
}

/// アプリ内の UI 言語(#44)。本体・Renderer 共通の単一ソース(本体は Renderer に依存するのでここに置く)。
/// - SwiftUI の `Text("…")` はルートで注入する `\.locale`(= `locale`)で即時に切り替わる
/// - `String` を返す文言は `string(_:bundle:)` を通す。`@Observable` なので body 内で呼べば切替時に再描画される
/// - 端末の `AppleLanguages` は書き換えない(システム UI・権限ダイアログは端末の言語のまま)
@Observable
public final class LanguagePreference {
    nonisolated(unsafe) public static let shared = LanguagePreference()
    static let storageKey = "metamon.appLanguage"
    /// 対応言語(ja が開発言語 = ソース文字列)
    public static let supported = ["ja", "en"]

    public var setting: AppLanguage {
        didSet { UserDefaults.standard.set(setting.rawValue, forKey: Self.storageKey) }
    }

    public init(defaults: UserDefaults = .standard) {
        setting = AppLanguage(rawValue: defaults.string(forKey: Self.storageKey) ?? "") ?? .system
    }

    /// 実際に使う言語コード("ja" | "en")。生成 API の `locale` と音声認識の言語にもこれを使う。
    public var code: String {
        switch setting {
        case .ja: "ja"
        case .en: "en"
        case .system: Self.systemCode
        }
    }

    /// 端末の優先言語から ja / en を選ぶ(どちらも無ければ開発言語の ja)。
    public static var systemCode: String {
        Bundle.preferredLocalizations(from: supported, forPreferences: Locale.preferredLanguages).first ?? "ja"
    }

    /// 書式(数値・日付)と `Text` の文言解決に使う Locale。言語が端末と同じなら地域設定ごと端末に従う。
    public var locale: Locale {
        let current = Locale.current
        if current.language.languageCode?.identifier == code { return current }
        return Locale(identifier: code == "ja" ? "ja_JP" : "en_US")
    }

    /// `base` 内の `<code>.lproj` を返す(無ければ base そのもの)。
    public func bundle(for base: Bundle) -> Bundle {
        if let path = base.path(forResource: code, ofType: "lproj"), let b = Bundle(path: path) { return b }
        return base
    }

    /// 現在の言語で文言を引く。キーは日本語のソース文字列。
    public func string(_ key: String.LocalizationValue, bundle: Bundle) -> String {
        String(localized: key, bundle: self.bundle(for: bundle), locale: locale)
    }
}

/// Renderer 内の固定文言(`Bundle.module` の String Catalog)。
func L(_ key: String.LocalizationValue) -> String {
    LanguagePreference.shared.string(key, bundle: .module)
}
