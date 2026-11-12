// 開発用メニュー(#2079)。`METAMON_STG` を立てた構成(Debug / Staging)にしか入らない。
// Release / Review は project.yml でこのフラグを足していないので、この下のコードはバイナリに 1 行も入らない。
#if METAMON_STG
import Foundation

/// 開発用メニューを出すかどうかの判定。**判定はここだけに置く**(呼ぶ側で条件を足さない)。
///
/// 二段構えにしている理由:
/// 1. `#if METAMON_STG`: 配布する版(Release / Review)にはコードごと入らない
/// 2. 接続先が prod なら出さない: Staging の構成のまま `METAMON_AI_BASE_URL=…-prod-…` で
///    ビルドして人に渡してしまった時の取りこぼしを防ぐ(構成だけでは守れない)
enum DebugMenuPolicy {
    /// prod の Cloud Run のホスト名の目印(`ai-prod-…` / `appconfig-prod-…`)。
    static let prodHostMarker = "-prod-"

    /// prod に向いた URL か。ホスト名で見る(nil = モック / インメモリなので prod ではない)。
    static func isProd(_ url: URL?) -> Bool {
        guard let host = url?.host()?.lowercased() else { return false }
        return host.contains(prodHostMarker)
    }

    /// どちらの接続先も prod でなければ出す。
    static func isAvailable(aiURL: URL?, appConfigURL: URL?) -> Bool {
        !isProd(aiURL) && !isProd(appConfigURL)
    }

    /// 今の起動での判定(`BuildConfig` の実効値 = 環境変数 → Info.plist の順)。
    static var isAvailable: Bool {
        isAvailable(aiURL: BuildConfig.aiBaseURL, appConfigURL: BuildConfig.appConfigBaseURL)
    }
}

/// メニューの先頭に出す「今の環境」の 1 行。
struct DebugEnvironmentRow: Identifiable, Equatable {
    let label: String
    /// 画面に出す値。空のときは `-`(`init` で揃える)
    let value: String
    /// コピーできる行か(uid・appId)
    let copyable: Bool

    init(_ label: String, _ value: String?, copyable: Bool = false) {
        self.label = label
        let trimmed = (value ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
        self.value = trimmed.isEmpty ? "-" : trimmed
        self.copyable = copyable
    }

    var id: String { label }
}

/// 今の環境の読み口。値は実行時に読むだけで、**ソースには一切持たない**(uid などを固定値で書かない)。
enum DebugEnvironment {
    /// ビルド構成。`METAMON_STG` が立つのは Debug と Staging だけで、DEBUG が立つのは Debug だけ。
    static var configuration: String {
        #if DEBUG
        "Debug"
        #else
        "Staging"
        #endif
    }

    /// Firebase の環境(`METAMON_FIREBASE_ENV` → Info.plist `MetamonFirebaseEnv`)。
    static func firebaseEnv(bundle: Bundle = .main,
                            environment: [String: String] = ProcessInfo.processInfo.environment) -> String? {
        let fromEnv = environment["METAMON_FIREBASE_ENV"]
        let fromPlist = bundle.object(forInfoDictionaryKey: "MetamonFirebaseEnv") as? String
        // 展開されていない `$(…)` は値ではない(xcodegen を通さずに開いた時)
        return [fromEnv, fromPlist].compactMap { $0 }.first { !$0.isEmpty && !$0.hasPrefix("$(") }
    }

    /// 画面に出す行を組む。引数で渡すのは、画面を持たないテストからも同じ並びを確かめられるようにするため。
    static func rows(configuration: String = Self.configuration,
                     version: String?, build: String?,
                     aiURL: URL?, appConfigURL: URL?, isLiveAppConfig: Bool,
                     firebaseEnv: String?, uid: String?, isAnonymous: Bool?, appId: String) -> [DebugEnvironmentRow] {
        [
            DebugEnvironmentRow("構成", configuration),
            DebugEnvironmentRow("版 / ビルド", "\(version ?? "-") (\(build ?? "-"))"),
            DebugEnvironmentRow("ai", aiURL?.absoluteString ?? "モック"),
            DebugEnvironmentRow("appconfig", appConfigURL.map { isLiveAppConfig ? $0.absoluteString : "インメモリ" } ?? "インメモリ"),
            DebugEnvironmentRow("Firebase", firebaseEnv),
            DebugEnvironmentRow("uid", uid ?? "未サインイン", copyable: true),
            DebugEnvironmentRow("アカウント", isAnonymous.map { $0 ? "匿名" : "Apple でサインイン" }),
            DebugEnvironmentRow("appId", appId, copyable: true),
        ]
    }

    /// コピー用のテキスト(「全部コピー」)。
    static func clipboardText(_ rows: [DebugEnvironmentRow]) -> String {
        rows.map { "\($0.label): \($0.value)" }.joined(separator: "\n")
    }
}
#endif
