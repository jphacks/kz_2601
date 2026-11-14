#if !APP_STORE_REVIEW // 審査用ビルドはプッシュの通信ごと外す(#269)
import CryptoKit
import Foundation

/// 端末トークンの登録(#269)。変わった時だけ送り、送れなければ次に前面へ来た時に送り直す。
public actor PushDeviceRegistrar {
    /// 端末で 1 つ。ログアウトでも変えない(`LocalWipe.preservedKeys`)
    public static let installationIdKey = "metamon.push.installationId"
    static let lastSentKey = "metamon.push.lastSent"

    private let client: any PushDeviceClient
    private let defaults: UserDefaults
    private let environment: PushEnvironment
    private let appVersion: String
    private let os: String
    private var sending: (fingerprint: String, task: Task<Bool, Never>)?
    private var unregistering = false

    public init(client: any PushDeviceClient, defaults: UserDefaults = .standard, environment: PushEnvironment, appVersion: String, os: String) {
        self.client = client
        self.defaults = defaults
        self.environment = environment
        self.appVersion = appVersion
        self.os = os
    }

    public func register(token: String, uid: String) async {
        guard !unregistering else { return }
        let id = installationId()
        let registration = PushDeviceRegistration(token: token, environment: environment, appVersion: appVersion, os: os)
        let fingerprint = Self.fingerprint(uid: uid, id: id, registration)
        guard defaults.string(forKey: Self.lastSentKey) != fingerprint, sending?.fingerprint != fingerprint else { return }
        sending?.task.cancel()
        let task = Task { [client] in
            do {
                try await client.putDevice(installationId: id, registration: registration)
                return true
            } catch {
                print("[Metamon] push: device not registered")
                return false
            }
        }
        sending = (fingerprint, task)
        let sent = await task.value
        guard sending?.fingerprint == fingerprint else { return } // 新しい登録か unregister に替わった
        sending = nil
        if sent { defaults.set(fingerprint, forKey: Self.lastSentKey) }
    }

    /// サインアウト・退会の前。送れなくても `timeout` で戻る(後片付けを止めない)。
    public func unregister(timeout: Duration = .seconds(3)) async {
        unregistering = true
        defer { unregistering = false }
        defaults.removeObject(forKey: Self.lastSentKey) // 退会を取りやめた時は次の前面で登録し直す
        let inFlight = sending?.task
        sending = nil
        inFlight?.cancel()
        guard let id = defaults.string(forKey: Self.installationIdKey) else { return } // 登録したことが無い
        await withTaskGroup(of: Void.self) { [client] group in
            group.addTask {
                _ = await inFlight?.value
                do { try await client.deleteDevice(installationId: id) } catch { print("[Metamon] push: device not deleted") }
            }
            group.addTask { try? await Task.sleep(for: timeout) }
            await group.next()
            group.cancelAll()
        }
    }

    /// 通知の許可が切られた: 登録済みの時だけ消す。
    public func authorizationRevoked() async {
        guard defaults.string(forKey: Self.lastSentKey) != nil else { return }
        await unregister()
    }

    private func installationId() -> String {
        if let id = defaults.string(forKey: Self.installationIdKey) { return id }
        let id = UUID().uuidString
        defaults.set(id, forKey: Self.installationIdKey)
        return id
    }

    /// 端末にトークンそのものを残さない。
    private static func fingerprint(uid: String, id: String, _ r: PushDeviceRegistration) -> String {
        let text = [uid, id, r.token, r.environment, r.appVersion, r.os].joined(separator: "\n")
        return SHA256.hash(data: Data(text.utf8)).map { String(format: "%02x", $0) }.joined()
    }
}
#endif
