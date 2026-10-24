import Foundation
import MetamonServices

/// 起動環境。`METAMON_API_BASE_URL`(環境変数 or Info.plist `MetamonAPIBaseURL`)が無ければ全モックで動く。
enum AppEnvironment {
    case mock
    case live(appConfigURL: URL, aiURL: URL)

    /// 環境変数 / Info.plist から判定した現在の環境。
    static var current: AppEnvironment {
        guard let api = BuildConfig.apiBaseURL, let ai = BuildConfig.aiBaseURL else { return .mock }
        return .live(appConfigURL: api, aiURL: ai)
    }
}

/// **DI の入口**。`Services` はここで1回だけ組み立て、`MetamonApp` が `AppRuntime` と SwiftUI Environment(`\.services`)に渡す。
/// Mock / 本番の差し替えはこのファイルだけで完結する(View / Runtime は protocol にしか依存しない)。
enum BuildConfig {
    static var apiBaseURL: URL? {
        let env = ProcessInfo.processInfo.environment["METAMON_API_BASE_URL"]
        let plist = Bundle.main.object(forInfoDictionaryKey: "MetamonAPIBaseURL") as? String
        guard let raw = [env, plist].compactMap({ $0 }).first(where: { !$0.isEmpty }) else { return nil }
        return URL(string: raw.hasSuffix("/") ? raw : raw + "/")
    }

    static var aiBaseURL: URL? {
        if let raw = ProcessInfo.processInfo.environment["METAMON_AI_BASE_URL"], !raw.isEmpty {
            return URL(string: raw.hasSuffix("/") ? raw : raw + "/")
        }
        return apiBaseURL
    }

    /// WhisperKit のモデル名。`METAMON_WHISPER_MODEL`(環境変数 or Info.plist `MetamonWhisperModel`)、既定 `small`(日本語精度優先・約500MB)。
    /// `tiny`/`base`/`small`/`large-v3` の短縮名は `openai_whisper-` が補われる。フル名(`openai_whisper-large-v3-v20240930_626MB` 等)はそのまま。
    static var whisperModel: String {
        let env = ProcessInfo.processInfo.environment["METAMON_WHISPER_MODEL"]
        let plist = Bundle.main.object(forInfoDictionaryKey: "MetamonWhisperModel") as? String
        return [env, plist].compactMap { $0 }.first { !$0.isEmpty } ?? "small"
    }

    @MainActor
    static func make(environment: AppEnvironment = .current) -> Services {
        switch environment {
        case .mock:
            return Services(
                appConfig: InMemoryAppConfigClient(seed: MockSeed.demoRecords()),
                ai: MockAIClient(),
                sync: NoopDocumentSync(),
                token: NoAuthTokenProvider(),
                transcriber: makeTranscriber(),
                notifications: UserNotificationScheduler(),
                isLive: false
            )
        case .live(let appConfigURL, let aiURL):
            // TODO: Firebase 導入後は FirebaseTokenProvider に差し替える
            let token = NoAuthTokenProvider()
            let appConfig = HTTPAppConfigClient(baseURL: appConfigURL, token: token)
            return Services(
                appConfig: appConfig,
                ai: HTTPAIClient(baseURL: aiURL, token: token),
                sync: PollingDocumentSync(client: appConfig),
                token: token,
                transcriber: makeTranscriber(),
                notifications: UserNotificationScheduler(),
                isLive: true
            )
        }
    }

    /// 実機 = WhisperKit(モック API でも音声認識は本物)。シミュレータは CoreML/ANE が使えず実用不可なのでモック(文字入力フォールバック)。
    @MainActor
    static func makeTranscriber() -> any SpeechTranscriber {
        #if targetEnvironment(simulator)
        return MockTranscriber()
        #else
        return WhisperKitRecognizer(model: whisperModel)
        #endif
    }
}
