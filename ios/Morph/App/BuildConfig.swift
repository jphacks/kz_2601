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
    @MainActor
    static func mock() -> Services {
        let seed: [String: [JSONValue]] = [
            "expense": [
                ["id": "1", "title": "ランチ", "amount": 980, "category": "food", "date": "2026-10-24"],
                ["id": "2", "title": "電車", "amount": 420, "category": "transport", "date": "2026-10-24"],
            ],
        ]
        return Services(
            appConfig: InMemoryAppConfigClient(seed: seed),
            ai: MockAIClient(),
            sync: NoopDocumentSync(),
            token: NoAuthTokenProvider(),
            native: UIKitNativeBridge(),
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
            native: UIKitNativeBridge(),
            transcriber: MockTranscriber(),
            isLive: true
        )
    }
}
