import Foundation
import MetamonDSL
import MetamonServices

#if !APP_STORE_REVIEW
extension AppRuntime {
    /// サインアウト・退会の前に端末の登録を消す(`wipeLocalData` はサーバーに送らないので、その前に呼ぶ)。
    func unregisterPushDevice() async { await PushHost.shared.unregister() }

    /// 開いているアプリの同意の控え。保存が無い = 受け取らない(#339)。
    var notifyConsentAllowed: Bool {
        guard let uid = services.auth.currentUser?.uid else { return false }
        return NotifyConsentStore().allowed(uid: uid, appId: document.appId)
    }

    /// 切り替えを出すか: 共有・公開の記録があり、作成者でない(作成者には同意なしで届く)。役割を確かめられない時は出す。
    func canSetNotifyConsent() async -> Bool {
        guard services.pushDevices != nil, dsl.entities.contains(where: \.isShared) else { return false }
        guard let membership = services.sharing as? any MembershipClient else { return true }
        return (try? await membership.members(appId: document.appId))?.role != .owner
    }

    /// 同意を送る。成功したら控えも変える。
    @discardableResult
    func setNotifyConsent(_ allowed: Bool, appId: String) async -> Bool {
        guard let client = services.pushDevices, let uid = services.auth.currentUser?.uid else { return false }
        do {
            try await client.setNotifyConsent(appId: appId, allowed: allowed)
            NotifyConsentStore().set(allowed, uid: uid, appId: appId)
            return true
        } catch {
            print("[Metamon] push: notify consent not saved appId=\(appId) allowed=\(allowed)")
            return false
        }
    }
}
#else
extension AppRuntime {
    func unregisterPushDevice() async {}
}
#endif
