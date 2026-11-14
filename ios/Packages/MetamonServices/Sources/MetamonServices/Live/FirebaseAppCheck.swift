import DeviceCheck
import Foundation
import FirebaseAppCheck
import FirebaseCore

/// App Check のプロバイダの選び方(#318)。アプリ本体が構成から決めて `FirebaseBootstrap.configureIfAvailable(appCheck:)` に渡す。
public enum AppCheckMode: Sendable {
    /// 実機の配布版: iOS は App Attest(非対応の端末は DeviceCheck)、Mac は DeviceCheck(#930)
    case deviceAttestation
    /// シミュレータ・Debug 構成・CI: デバッグ用のトークン(Firebase コンソールに登録したものだけ通る)。
    /// 値は初回起動時にコンソールへ出る(`Firebase App Check debug token: …`)か、環境変数 `FIRAAppCheckDebugToken` で固定する
    case debug
    /// App Check を使わない(ヘッダを付けない)
    case off
}

/// iOS: App Attest を優先し、使えない端末(`DCAppAttestService.isSupported == false`)は DeviceCheck に落とす。
/// Mac(#930): App Attest は Mac では `isSupported == false` なので、最初から DeviceCheck(macOS 10.15 から使える)。
/// docs/infra/app-check.md「Mac のプロバイダ」
final class AttestationProviderFactory: NSObject, AppCheckProviderFactory {
    func createProvider(with app: FirebaseApp) -> (any AppCheckProvider)? {
        #if os(macOS)
        return DeviceCheckProvider(app: app)
        #else
        if DCAppAttestService.shared.isSupported, let attest = AppAttestProvider(app: app) {
            return attest
        }
        return DeviceCheckProvider(app: app)
        #endif
    }
}

enum FirebaseAppCheckSetup {
    /// `FirebaseApp.configure` より前に呼ぶ(Firebase の決まり)。
    static func installFactory(_ mode: AppCheckMode) {
        switch mode {
        case .deviceAttestation: AppCheck.setAppCheckProviderFactory(AttestationProviderFactory())
        case .debug: AppCheck.setAppCheckProviderFactory(AppCheckDebugProviderFactory())
        case .off: break
        }
    }
}

/// `AppCheck.appCheck()` からトークンを取る。SDK がキャッシュと期限前の更新を持つので毎リクエスト呼んでよい。
/// 初回の App Attest は Apple と Firebase への往復があるので、`timeout` を超えたら nil(ヘッダ無し)で先に進む。
public struct FirebaseAppCheckTokenProvider: AppCheckTokenProvider {
    public var timeout: TimeInterval

    public init(timeout: TimeInterval = 5) { self.timeout = timeout }

    public func token(forceRefresh: Bool) async -> String? {
        await firstValue(timeout: timeout) { done in
            AppCheck.appCheck().token(forcingRefresh: forceRefresh) { token, error in
                if let error { print("[Metamon] App Check: token unavailable: \(error.localizedDescription)") }
                done(token?.token)
            }
        }
    }
}

/// コールバック型の処理と時間切れのうち先に終わった方の値を返す(SDK の completion はキャンセルに反応しないため)。
func firstValue(timeout: TimeInterval, _ start: @escaping (@escaping @Sendable (String?) -> Void) -> Void) async -> String? {
    await withCheckedContinuation { (continuation: CheckedContinuation<String?, Never>) in
        let once = ResumeOnce(continuation)
        start { once.resume($0) }
        Task {
            try? await Task.sleep(for: .seconds(timeout))
            once.resume(nil)
        }
    }
}

private final class ResumeOnce: @unchecked Sendable {
    private let lock = NSLock()
    private var continuation: CheckedContinuation<String?, Never>?

    init(_ continuation: CheckedContinuation<String?, Never>) { self.continuation = continuation }

    func resume(_ value: String?) {
        let c: CheckedContinuation<String?, Never>? = lock.withLock {
            defer { continuation = nil }
            return continuation
        }
        c?.resume(returning: value)
    }
}
