import Foundation
import MetamonServices

/// UI テスト・ヘッドレス E2E 用(DEBUG のみ): `METAMON_AI_CONSENT=granted` で同意済み、`=reset` で初回(未同意・説明未表示)の状態で起動する。
enum AIConsentDebug {
    @MainActor
    static func applyLaunchOverride() {
        #if DEBUG
        switch ProcessInfo.processInfo.environment["METAMON_AI_CONSENT"] {
        case "granted":
            if !AIConsentStore.shared.isGranted { AIConsentStore.shared.grant() }
            AIConsentStore.shared.markPrompted()
        case "reset":
            AIConsentStore.shared.resetForTesting()
        default:
            break
        }
        #endif
    }
}
