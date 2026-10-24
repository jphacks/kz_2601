import SwiftUI
import MetamonServices

/// `Services` を SwiftUI Environment で配る。`MetamonApp` が `BuildConfig.make(environment:)` の結果を1回だけ注入し、
/// View は `@Environment(\.services)` で protocol 越しに使う(例: `VoiceInputView` の `services.transcriber`)。
private struct ServicesKey: EnvironmentKey {
    static let defaultValue: Services? = nil
}

extension EnvironmentValues {
    var services: Services {
        get {
            guard let s = self[ServicesKey.self] else {
                preconditionFailure("Services が未注入です。RootView に .environment(\\.services, BuildConfig.make()) を付けてください")
            }
            return s
        }
        set { self[ServicesKey.self] = newValue }
    }
}
