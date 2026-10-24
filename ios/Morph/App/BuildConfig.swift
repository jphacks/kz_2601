import Foundation

/// 起動構成。`MORPH_API_BASE_URL`(環境変数 or Info.plist `MorphAPIBaseURL`)が無ければ全モックで動く。
enum BuildConfig {
    static var apiBaseURL: URL? {
        let env = ProcessInfo.processInfo.environment["MORPH_API_BASE_URL"]
        let plist = Bundle.main.object(forInfoDictionaryKey: "MorphAPIBaseURL") as? String
        guard let raw = [env, plist].compactMap({ $0 }).first(where: { !$0.isEmpty }) else { return nil }
        return URL(string: raw.hasSuffix("/") ? raw : raw + "/")
    }

    static var aiBaseURL: URL? {
        if let raw = ProcessInfo.processInfo.environment["MORPH_AI_BASE_URL"], !raw.isEmpty {
            return URL(string: raw.hasSuffix("/") ? raw : raw + "/")
        }
        return apiBaseURL
    }

    @MainActor
    static func makeServices() -> Services {
        guard let api = apiBaseURL, let ai = aiBaseURL else { return .mock() }
        return .live(appConfigURL: api, aiURL: ai)
    }
}

extension Services {
    /// デモ3本(docs/dsl/examples)の entity id に合わせた seed データ。
    @MainActor
    static func mock() -> Services {
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
            transcriber: MockTranscriber(),
            isLive: false
        )
    }

    @MainActor
    static func live(appConfigURL: URL, aiURL: URL) -> Services {
        // TODO: Firebase 導入後は FirebaseTokenProvider / WhisperKitTranscriber に差し替える
        let token = NoAuthTokenProvider()
        let appConfig = HTTPAppConfigClient(baseURL: appConfigURL, token: token)
        return Services(
            appConfig: appConfig,
            ai: HTTPAIClient(baseURL: aiURL, token: token),
            sync: PollingDocumentSync(client: appConfig),
            token: token,
            transcriber: MockTranscriber(),
            isLive: true
        )
    }
}
