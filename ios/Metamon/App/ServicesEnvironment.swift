import SwiftUI
import MetamonServices

/// `Services` を SwiftUI Environment で配る。`MetamonApp` が `BuildConfig.make(environment:)` の結果を1回だけ注入し、
/// View は `@Environment(\.services)` で protocol 越しに使う(例: `VoiceInputView` の `services.transcriber`)。
///
/// 注意: `.environment(\.services, x)` は WritableKeyPath 経由で **getter を呼んでから setter を呼ぶ**ので、
/// getter で `preconditionFailure` すると注入そのものがクラッシュする。未注入時は軽量なモック束(placeholder)を返す。
private struct ServicesKey: EnvironmentKey {
    static let defaultValue: Services? = nil

    /// 未注入時のフォールバック(全モック・WhisperKit 等の重い依存は持たない)。
    static let placeholder: Services = MainActor.assumeIsolated {
        Services(
            auth: MockAuthService(signedIn: true),
            appConfig: InMemoryAppConfigClient(),
            ai: MockAIClient(),
            sync: NoopDocumentSync(),
            token: NoAuthTokenProvider(),
            transcriber: MockTranscriber(),
            notifications: UserNotificationScheduler(),
            isLive: false
        )
    }
}

extension EnvironmentValues {
    var services: Services {
        get { self[ServicesKey.self] ?? ServicesKey.placeholder }
        set { self[ServicesKey.self] = newValue }
    }
}
