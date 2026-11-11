import Foundation
import MetamonServices

/// Mac の Firebase の初期化(#930)。iOS の `MetamonApp.init` の先頭と同じ順: Firebase(App Check 込み)→ クラッシュ報告。
/// `GoogleService-Info.plist` がバンドルに無い時(CI・plist 未同期)は何もせず、接続はモックのまま(iOS と同じ)。
enum MacFirebase {
    /// App Check(#318・#930): Debug 構成はデバッグ用のトークン、Staging / Release / Review は DeviceCheck
    /// (Mac は App Attest が使えない。`AttestationProviderFactory` が Mac では DeviceCheck を選ぶ)。docs/infra/app-check.md
    static var appCheckMode: AppCheckMode {
        #if DEBUG
        .debug
        #else
        .deviceAttestation
        #endif
    }

    /// アプリの起動の最初に 1 回(`MacServices` を組み立てる前)
    @MainActor
    static func bootstrap() {
        // Mac の Crashlytics は、捕まえていない例外でアプリを落とすようにしないと報告が出ない(Firebase の決まり)
        UserDefaults.standard.register(defaults: ["NSApplicationCrashOnExceptions": true])
        FirebaseBootstrap.configureIfAvailable(appCheck: appCheckMode)
        CrashReportingPreference.shared.activate(reporter: FirebaseCrashReporter() ?? NoopCrashReporter())
    }
}

/// Mac の DI の入口(#930)。iOS の `BuildConfig.make` の Mac 版で、Mac が使う分(ログイン・AI・フィードバック)だけ組み立てる。
/// Firebase が初期化済み・ai の URL がある・`METAMON_MOCK=1` でない時だけ本物。それ以外はモック(今までの Mac と同じ)。
/// appconfig(記録・版・共有など)は、本物の AI に加えて appconfig の URL がある時だけ本物(`appConfig`)。
/// 無い時、作ったアプリの記録は端末だけ(`MacRecordVault`・#1517 の `LocalAppConfigClient`)。
final class MacServices: @unchecked Sendable {
    static let shared = MacServices.make()

    let auth: any AuthService
    let token: any TokenProvider
    /// 同意の包み(`MacComposeSession.gatedAI`)の中身
    let ai: any AIClient
    let feedback: any FeedbackClient
    /// Firebase につながっている(サインイン・本物の AI)。false ならサインインの入口を出さない
    let isLive: Bool
    /// サーバーの appconfig(#1284 の続き)。本物の AI が作ったアプリはサーバーに保存されるので、その記録はここに読み書きする。
    /// nil(モック・appconfig の URL 無し)なら作ったアプリは端末だけ
    let appConfig: (any AppConfigClient)?
    /// appconfig の URL(本の情報・feed など appconfig の別の口が使う)。`appConfig` と同じく live の時だけ
    let appConfigURL: URL?

    init(auth: any AuthService, ai: any AIClient, feedback: any FeedbackClient, isLive: Bool, appConfigURL: URL? = nil,
         token: (any TokenProvider)? = nil) {
        self.auth = auth
        let token = token ?? (isLive ? AuthTokenProvider(auth: auth) : NoAuthTokenProvider())
        self.token = token
        self.ai = ai
        self.feedback = feedback
        self.isLive = isLive
        let url = isLive ? appConfigURL : nil
        self.appConfigURL = url
        appConfig = url.map { HTTPAppConfigClient(baseURL: $0, token: token) }
    }

    #if !APP_STORE_REVIEW
    /// 外部の公開データ(feed・#348・#1288)。appconfig の URL がある時だけ(審査用ビルドは外す・iOS と同じ)
    var feeds: (any FeedClient)? { appConfigURL.map { HTTPFeedClient(baseURL: $0, token: token) } }
    #endif

    /// 作ったアプリをサーバーに置くか(本物の AI と appconfig の両方がある)。false なら記録は端末だけ
    var keepsAppsOnServer: Bool { appConfig != nil }

    static func make(bundle: Bundle = .main, environment: [String: String] = ProcessInfo.processInfo.environment) -> MacServices {
        let forceMock = environment["METAMON_MOCK"] == "1"
        if !forceMock, let auth = FirebaseAuthService(), let aiURL = url(env: "METAMON_AI_BASE_URL", plist: "MetamonAIBaseURL", bundle: bundle, environment: environment) {
            let token = AuthTokenProvider(auth: auth)
            let appConfigURL = url(env: "METAMON_APPCONFIG_BASE_URL", plist: "MetamonAppConfigBaseURL", bundle: bundle, environment: environment)
            #if APP_STORE_REVIEW
            let feedback: any FeedbackClient = MockFeedbackClient()
            #else
            let feedback: any FeedbackClient = appConfigURL.map { HTTPFeedbackClient(baseURL: $0, token: token) } ?? MockFeedbackClient()
            #endif
            return MacServices(auth: auth, ai: HTTPAIClient(baseURL: aiURL, token: token), feedback: feedback, isLive: true,
                               appConfigURL: appConfigURL, token: token)
        }
        if !forceMock { print("[Metamon] Mac: Firebase 未初期化か ai の URL が無いため、AI とログインはモックで起動します") }
        return MacServices(auth: MockAuthService(signedIn: true), ai: MockAIClient(), feedback: MockFeedbackClient(), isLive: false)
    }

    /// iOS の `BuildConfig.url` と同じ読み方(環境変数 → Info.plist。空・展開されていない `$(…)` は無し)
    static func url(env: String, plist: String, bundle: Bundle = .main,
                    environment: [String: String] = ProcessInfo.processInfo.environment) -> URL? {
        let fromPlist = bundle.object(forInfoDictionaryKey: plist) as? String
        guard let raw = [environment[env], fromPlist].compactMap({ $0 }).first(where: { !$0.isEmpty && !$0.hasPrefix("$(") }) else { return nil }
        return URL(string: raw.hasSuffix("/") ? raw : raw + "/")
    }
}
