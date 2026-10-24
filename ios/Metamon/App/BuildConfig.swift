import Foundation
import MetamonDSL

/// 起動構成。`METAMON_API_BASE_URL`(環境変数 or Info.plist `MetamonAPIBaseURL`)が無ければ全モックで動く。
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
    static func makeServices() -> Services {
        guard let api = apiBaseURL, let ai = aiBaseURL else { return .mock(transcriber: makeTranscriber()) }
        return .live(appConfigURL: api, aiURL: ai, transcriber: makeTranscriber())
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

extension Services {
    /// デモ3本(docs/dsl/examples)の entity id に合わせた seed データ。
    @MainActor
    static func mock(transcriber: (any SpeechTranscriber)? = nil) -> Services {
        let today = FieldCoding.dateFormatter.string(from: .now)
        let now = ISO8601DateFormatter().string(from: .now)
        let seed: [String: [Record]] = [
            "expense": [
                Record(id: "1", data: ["amount": 980, "category": "食費", "memo": "ランチ", "date": .string(today)], createdAt: now),
                Record(id: "2", data: ["amount": 420, "category": "交通", "memo": "電車", "date": .string(today)], createdAt: now),
            ],
            "workout": [
                Record(id: "1", data: ["exercise": "スクワット", "reps": 20, "weight": 40, "done": true, "date": .string(now)], createdAt: now),
            ],
            "memo": [
                Record(id: "1", data: ["note": "きれいな紅葉", "date": .string(now)], createdAt: now),
            ],
        ]
        return Services(
            appConfig: InMemoryAppConfigClient(seed: seed),
            ai: MockAIClient(),
            sync: NoopDocumentSync(),
            token: NoAuthTokenProvider(),
            transcriber: transcriber ?? MockTranscriber(),
            isLive: false
        )
    }

    @MainActor
    static func live(appConfigURL: URL, aiURL: URL, transcriber: any SpeechTranscriber) -> Services {
        // TODO: Firebase 導入後は FirebaseTokenProvider に差し替える
        let token = NoAuthTokenProvider()
        let appConfig = HTTPAppConfigClient(baseURL: appConfigURL, token: token)
        return Services(
            appConfig: appConfig,
            ai: HTTPAIClient(baseURL: aiURL, token: token),
            sync: PollingDocumentSync(client: appConfig),
            token: token,
            transcriber: transcriber,
            isLive: true
        )
    }
}
