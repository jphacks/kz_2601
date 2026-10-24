import Foundation
import Observation
import MetamonServices

/// 課金(#77)の司令塔。無料枠(端末で数える)+ Pro の権利(StoreKit 2)+ 上限シートの表示状態。
/// AppRuntime からは生成の入口で `admitNewApp()` / `recordNewApp()` だけを呼ぶ。
@MainActor
@Observable
final class ProGate {
    static let shared = ProGate()

    let store: ProEntitlementStore
    @ObservationIgnored let usage: CreationUsageStore
    let quota: CreationQuota
    /// 上限に達した時のシート
    var paywallPresented = false
    /// 今日の新規作成回数(`usage` の写し。表示の再描画用)
    private(set) var usedToday: Int

    /// デモ端末用: 回数制限を無効にする(DEBUG ビルドのみ設定に出す。既定 ON = デモで上限に当たらない)。
    /// `METAMON_IAP_ENFORCE=1` を付けて起動すると設定に関係なく制限を有効にする(上限シートの確認用)。
    var limitDisabled: Bool {
        didSet { UserDefaults.standard.set(limitDisabled, forKey: Self.disabledKey) }
    }

    private static let disabledKey = "metamon.pro.limitDisabled"

    init(store: ProEntitlementStore? = nil,
         usage: CreationUsageStore = CreationUsageStore(),
         quota: CreationQuota = CreationQuota(dailyFreeLimit: ProGate.configuredLimit)) {
        self.store = store ?? ProEntitlementStore()
        self.usage = usage
        self.quota = quota
        self.usedToday = usage.used()
        self.limitDisabled = (UserDefaults.standard.object(forKey: Self.disabledKey) as? Bool) ?? Self.defaultLimitDisabled
    }

    /// 起動時に 1 回(`PaywallHost` の task)。
    func start() {
        #if DEBUG
        StoreKitTestHarness.startIfRequested()
        #endif
        store.start()
    }

    var isPro: Bool { store.isPro }
    var isEnforced: Bool { Self.forceEnforce || !limitDisabled }

    var decision: CreationQuota.Decision {
        quota.decide(used: usedToday, isPro: store.isPro, limitDisabled: !isEnforced)
    }

    /// 新規作成の前に呼ぶ。枠切れなら上限シートを出して false(呼び出し側は送らない)。
    /// 音声シートなど別のシートが閉じ終わるのを待ってから出す。
    func admitNewApp() -> Bool {
        usedToday = usage.used()
        guard decision == .blocked else { return true }
        print("[Metamon] IAP: daily free limit reached (\(usedToday)/\(quota.dailyFreeLimit)) → paywall")
        Task {
            try? await Task.sleep(for: .milliseconds(450))
            paywallPresented = true
        }
        return false
    }

    /// 新規作成が成功した時に呼ぶ(失敗・キャンセルは数えない)。Pro / 無効化中も記録だけはする。
    func recordNewApp() {
        usage.record()
        usedToday = usage.used()
        print("[Metamon] IAP: creations today=\(usedToday) pro=\(store.isPro) enforced=\(isEnforced)")
    }

    // MARK: - 起動時の設定

    /// DEBUG ビルドは既定で制限なし(デモ端末で上限に当たらない)。Release は既定で制限あり。
    private static var defaultLimitDisabled: Bool {
        #if DEBUG
        true
        #else
        false
        #endif
    }

    private static var forceEnforce: Bool {
        ProcessInfo.processInfo.environment["METAMON_IAP_ENFORCE"] == "1"
    }

    /// `METAMON_IAP_FREE_LIMIT=<n>` で 1 日の無料回数を上書き(確認用)。
    nonisolated static var configuredLimit: Int {
        ProcessInfo.processInfo.environment["METAMON_IAP_FREE_LIMIT"].flatMap(Int.init) ?? ProProducts.dailyFreeCreations
    }
}
