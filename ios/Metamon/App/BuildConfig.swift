import Foundation
import MetamonDSL
import MetamonServices
import MetamonRenderer

/// 起動環境。appconfig / ai の URL は別々に設定でき、どちらも無ければ全モックで動く。
/// 既定(project.yml)は ai = staging・appconfig = staging(どちらも Cloud Run https)。
/// - `METAMON_AI_BASE_URL`(env) / `MetamonAIBaseURL`(Info.plist): ai サービス(`POST /v1/generate`)
/// - `METAMON_APPCONFIG_BASE_URL`(env) / `MetamonAppConfigBaseURL`(Info.plist): appconfig(DSL/records CRUD・ホットリロード)
/// Info.plist の値は build setting `$(METAMON_*_BASE_URL)` から展開されるので、`xcodebuild ... METAMON_AI_BASE_URL=http://…` でも渡せる。
enum AppEnvironment {
    case mock
    /// nil の側はインメモリ/モックのまま(例: ai だけ live、appconfig は未実装なのでインメモリ)。
    case live(appConfigURL: URL?, aiURL: URL?)

    static var current: AppEnvironment {
        if BuildConfig.forceMock { return .mock }
        let appConfig = BuildConfig.appConfigBaseURL
        let ai = BuildConfig.aiBaseURL
        if appConfig == nil && ai == nil { return .mock }
        return .live(appConfigURL: appConfig, aiURL: ai)
    }
}

/// **DI の入口**。`Services` はここで1回だけ組み立て、`MetamonApp` が `AppRuntime` と SwiftUI Environment(`\.services`)に渡す。
/// Mock / 本番の差し替えはこのファイルだけで完結する(View / Runtime は protocol にしか依存しない)。
enum BuildConfig {
    /// ウィジェットと共有する App Group(entitlements と一致。実体は両ターゲット共通の `WidgetSnapshot`)。
    static let appGroupID = WidgetSnapshot.appGroupID

    /// `METAMON_MOCK=1`(Scheme の環境変数)で URL 設定を無視して全モック起動(オフラインでの UI 確認用)。
    static var forceMock: Bool {
        ProcessInfo.processInfo.environment["METAMON_MOCK"] == "1"
    }

    static var appConfigBaseURL: URL? {
        url(env: "METAMON_APPCONFIG_BASE_URL", plist: "MetamonAppConfigBaseURL")
            ?? url(env: "METAMON_API_BASE_URL", plist: "MetamonAPIBaseURL") // 旧名
    }

    static var aiBaseURL: URL? {
        url(env: "METAMON_AI_BASE_URL", plist: "MetamonAIBaseURL")
    }

    private static func url(env: String, plist: String) -> URL? {
        let fromEnv = ProcessInfo.processInfo.environment[env]
        let fromPlist = Bundle.main.object(forInfoDictionaryKey: plist) as? String
        guard let raw = [fromEnv, fromPlist].compactMap({ $0 }).first(where: { !$0.isEmpty && !$0.hasPrefix("$(") }) else { return nil }
        return URL(string: raw.hasSuffix("/") ? raw : raw + "/")
    }

    /// WhisperKit のモデル名。`METAMON_WHISPER_MODEL`(環境変数 or Info.plist `MetamonWhisperModel`)、既定 `small`(日本語精度優先・約500MB)。
    /// `tiny`/`base`/`small`/`large-v3` の短縮名は `openai_whisper-` が補われる。フル名(`openai_whisper-large-v3-v20240930_626MB` 等)はそのまま。
    static var whisperModel: String {
        let env = ProcessInfo.processInfo.environment["METAMON_WHISPER_MODEL"]
        let plist = Bundle.main.object(forInfoDictionaryKey: "MetamonWhisperModel") as? String
        return [env, plist].compactMap { $0 }.first { !$0.isEmpty } ?? "small"
    }

    /// モックの初期レコード。DEBUG の見た目監査(#131)では DSL の entity からダミーを作る。
    private static var mockSeed: [String: [Record]] {
        #if DEBUG
        if let seed = DebugDSLOverride.seed { return seed }
        #endif
        return MockSeed.demoRecords()
    }

    @MainActor
    static func make(environment: AppEnvironment = .current) -> Services {
        switch environment {
        case .mock:
            return Services(
                auth: MockAuthService(signedIn: true),
                appConfig: InMemoryAppConfigClient(seed: mockSeed),
                ai: MockAIClient(),
                sync: NoopDocumentSync(),
                token: NoAuthTokenProvider(),
                transcriber: makeTranscriber(),
                notifications: UserNotificationScheduler(),
                isLive: false
            )
        case .live(let appConfigURL, let aiURL):
            let auth = makeAuth()
            let token: any TokenProvider = devToken ?? AuthTokenProvider(auth: auth)
            let appConfig: any AppConfigClient = appConfigURL.map { HTTPAppConfigClient(baseURL: $0, token: token) }
                ?? InMemoryAppConfigClient(seed: MockSeed.demoRecords())
            let sync: any DocumentSync = (appConfig as? HTTPAppConfigClient).map(makeSync) ?? NoopDocumentSync()
            var services = Services(
                auth: auth,
                appConfig: appConfig,
                ai: aiURL.map { HTTPAIClient(baseURL: $0, token: token) } ?? MockAIClient(),
                sync: sync,
                token: token,
                transcriber: makeTranscriber(),
                notifications: UserNotificationScheduler(),
                isLive: appConfigURL != nil
            )
            services.sharing = appConfigURL.map { HTTPSharingClient(baseURL: $0, token: token) }
            if let appConfigURL { services.account = HTTPAccountClient(baseURL: appConfigURL, token: token) } // #119
            services.releaseRequests = appConfigURL.map { HTTPReleaseRequestClient(baseURL: $0, token: token) }
            return services
        }
    }

    /// ホットリロード: SSE(`GET /events`)+ ポーリングのフォールバック(#82)。
    /// DEBUG のみ `METAMON_REALTIME=0` で Day1 のポーリングだけに戻せる(比較・切り分け用)。
    static func makeSync(_ client: HTTPAppConfigClient) -> any DocumentSync {
        #if DEBUG
        if ProcessInfo.processInfo.environment["METAMON_REALTIME"] == "0" {
            print("[Metamon] METAMON_REALTIME=0: ポーリングのみ(5 秒)")
            return PollingDocumentSync(client: client)
        }
        #endif
        return RealtimeDocumentSync(client: client)
    }

    /// DEBUG のみ: `METAMON_DEV_UID=<uid>` で Bearer にその文字列を送る。`AUTH_MODE=dev` のローカル backend は Bearer をそのまま uid として扱うので、
    /// シミュレータ E2E で端末固定の uid を使える(Firebase の ID トークンは 1 時間で変わり、dev モードでは別ユーザーに見えてしまう)。
    static var devToken: (any TokenProvider)? {
        #if DEBUG
        if let uid = ProcessInfo.processInfo.environment["METAMON_DEV_UID"], !uid.isEmpty {
            print("[Metamon] METAMON_DEV_UID=\(uid): Bearer に固定 uid を送ります(ローカル AUTH_MODE=dev 専用)")
            return StaticTokenProvider(uid)
        }
        #endif
        return nil
    }

    /// Firebase が初期化済みなら本物、plist 未同期なら警告してモック(固定 uid・Bearer 無し)。
    static func makeAuth() -> any AuthService {
        if let firebase = FirebaseAuthService() { return firebase }
        print("[Metamon] Firebase 未初期化のため MockAuthService で起動します(Authorization ヘッダは付きません)")
        return MockAuthService(signedIn: true)
    }

    /// 実機 = WhisperKit(モック API でも音声認識は本物)。シミュレータは CoreML/ANE が使えず実用不可なのでモック(文字入力フォールバック)。
    @MainActor
    static func makeTranscriber() -> any SpeechTranscriber {
        #if targetEnvironment(simulator)
        return MockTranscriber()
        #else
        let recognizer = WhisperKitRecognizer(model: whisperModel)
        recognizer.language = { LanguagePreference.shared.code } // 認識言語は UI 言語に合わせる(#45)
        return recognizer
        #endif
    }
}

/// 端末ごとに固定の appId(初回起動で UUID を 1 回生成し UserDefaults に保存)。
/// ai/appconfig の DSL とレコードはこのキーに紐付く。サーバーが知らない appId は「初期状態」として扱われる。
enum DeviceAppID {
    private static let key = "metamon.appId"

    static var current: String {
        if let saved = UserDefaults.standard.string(forKey: key), !saved.isEmpty { return saved }
        let fresh = UUID().uuidString.lowercased()
        UserDefaults.standard.set(fresh, forKey: key)
        return fresh
    }

    /// 「アプリをリセット」用: 新しい appId を発行して保存する(サーバーからは新規端末に見える)。
    @discardableResult
    static func rotate() -> String {
        let fresh = UUID().uuidString.lowercased()
        UserDefaults.standard.set(fresh, forKey: key)
        return fresh
    }

    /// 変身一覧からの切替用: 既存のアプリ(`GET /apps` の id)を現在のアプリにする。
    static func set(_ appId: String) {
        UserDefaults.standard.set(appId, forKey: key)
    }
}
