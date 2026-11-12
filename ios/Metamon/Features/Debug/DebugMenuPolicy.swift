import Foundation

/// 開発用メニュー(#2079)を出すかの判定。ここだけで決める。
/// メニューの中身(画面・つまみ・各セクション)は `#if METAMON_STG` の中にあり、Release / Review の版には入らない。
/// この型だけは全部の版に入る(判定の単体テストを Release の条件でも書けるように。文言は持たない)。
enum DebugMenuPolicy {
    /// この版にメニューのコードが入っているか(Debug / Staging の構成だけ `METAMON_STG` が立つ・project.yml)
    static var isCompiledIn: Bool {
        #if METAMON_STG
        true
        #else
        false
        #endif
    }

    /// 出すか。Staging の構成で prod に向けてビルドした時の取りこぼしを防ぐため、接続先が prod なら出さない
    static var isAvailable: Bool {
        isAvailable(compiledIn: isCompiledIn, aiURL: BuildConfig.aiBaseURL, appConfigURL: BuildConfig.appConfigBaseURL)
    }

    static func isAvailable(compiledIn: Bool, aiURL: URL?, appConfigURL: URL?) -> Bool {
        compiledIn && !isProd(aiURL) && !isProd(appConfigURL)
    }

    /// prod の Cloud Run の URL(`ai-prod-…` / `appconfig-prod-…`)か。URL が無い(モック)なら prod ではない
    static func isProd(_ url: URL?) -> Bool {
        url?.absoluteString.lowercased().contains("-prod-") ?? false
    }
}
