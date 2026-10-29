// 審査用ビルド(APP_STORE_REVIEW・#450 G1)は App 内課金を持たない(課金画面・端末の無料枠・StoreKit の監視をしない)。
#if !APP_STORE_REVIEW
import Foundation
import Observation
import MetamonServices

/// 課金画面を開く理由(#414)。見出しと「何が使えるようになるか」の先頭を変える。
enum PaywallReason: Equatable {
    /// 設定の「Pro にする」
    case settings
    /// 今日の無料作成を使い切った
    case limitReached
    /// 共有・オンラインなどの有料機能を使うアプリを作る / 直す(features が空なら「みんなで共有するデータ」)
    case paidFeature([PaidFeature])
    /// 記録を共有するアプリ・公開アプリに参加する
    case join(appName: String)
}

/// 課金画面の表示要求。`placement` = どこの View が present するか(チャットの確認カードは音声シートの中にもあるので、その場で出す)。
struct PaywallRequest: Identifiable, Equatable {
    enum Placement: Equatable { case root, chat }

    let id = UUID()
    var reason: PaywallReason
    var placement: Placement
    /// 閉じた(買わなかった)時にどうなるかの一言
    var closeNote: String?
}

/// 課金(#77)の司令塔。無料枠(端末で数える)+ Pro の権利(StoreKit 2)+ 課金画面の表示状態。
/// AppRuntime からは生成の入口で `admitNewApp()` / `recordNewApp()`、有料機能の入口で `requestPro(_:)` を呼ぶ(#414)。
@MainActor
@Observable
final class ProGate {
    static let shared = ProGate()

    let store: ProEntitlementStore
    @ObservationIgnored let usage: CreationUsageStore
    let quota: CreationQuota
    /// いま出している課金画面(nil = 出していない)
    private(set) var paywall: PaywallRequest?
    @ObservationIgnored private var paywallWaiter: CheckedContinuation<Bool, Never>?
    /// 課金画面を出しているか(上限シートのテスト用)
    var paywallPresented: Bool { paywall != nil }
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
         quota: CreationQuota = CreationQuota(dailyFreeLimit: ProGate.configuredLimit),
         limitDisabled: Bool? = nil) {
        self.store = store ?? ProEntitlementStore(mode: Self.storeMode)
        self.usage = usage
        self.quota = quota
        self.usedToday = usage.used()
        self.limitDisabled = limitDisabled
            ?? (UserDefaults.standard.object(forKey: Self.disabledKey) as? Bool) ?? Self.defaultLimitDisabled
    }

    /// 起動時に 1 回(`PaywallHost` の task)。
    func start() {
        store.start()
    }

    var isPro: Bool { store.isPro }
    var isEnforced: Bool { Self.forceEnforce || !limitDisabled }
    /// 有料機能の前に課金画面を出すか(`PaidGate.needsPaywall`)
    var needsPaywall: Bool { PaidGate.needsPaywall(isPro: isPro, enforced: isEnforced) }

    var decision: CreationQuota.Decision {
        quota.decide(used: usedToday, isPro: store.isPro, limitDisabled: !isEnforced)
    }

    /// 新規作成の前に呼ぶ。枠切れなら上限シートを出して false(呼び出し側は送らない)。
    /// 音声シートなど別のシートが閉じ終わるのを待ってから出す。
    func admitNewApp() -> Bool {
        usedToday = usage.used()
        guard decision == .blocked else { return true }
        print("[Metamon] IAP: daily free limit reached (\(usedToday)/\(quota.dailyFreeLimit)) → paywall")
        Task { _ = await requestPro(.limitReached, after: .milliseconds(450)) }
        return false
    }

    /// 課金画面を出し、閉じるまで待つ。戻り値 = 閉じた時に Pro か(買った / 復元した)。Pro ならすぐ true。
    /// 別のシートが閉じ終わるのを待つ時は `after` を付ける。前の課金画面が開いていれば、それは閉じた扱いにする。
    func requestPro(_ reason: PaywallReason, placement: PaywallRequest.Placement = .root, closeNote: String? = nil,
                    after delay: Duration = .zero) async -> Bool {
        if isPro { return true }
        if delay > .zero { try? await Task.sleep(for: delay) }
        if isPro { return true }
        finishPaywall()
        print("[Metamon] IAP: paywall reason=\(reason) placement=\(placement)")
        return await withCheckedContinuation { continuation in
            paywallWaiter = continuation
            paywall = PaywallRequest(reason: reason, placement: placement, closeNote: closeNote)
        }
    }

    /// 課金画面が閉じた(シートの onDismiss・購入後の自動クローズ・呼び出し側の取り消し)。待っている側に Pro かを返す。
    func finishPaywall() {
        let waiter = paywallWaiter
        paywallWaiter = nil
        paywall = nil
        waiter?.resume(returning: isPro)
    }

    /// `placement` の課金画面だけを閉じる(確認カードを閉じた時など)。
    func dismissPaywall(placement: PaywallRequest.Placement) {
        guard paywall?.placement == placement else { return }
        finishPaywall()
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

    /// StoreKit かモックか(#414)。DEBUG の `METAMON_MOCK_STORE=1` で常にモック(UI テスト)、
    /// DEBUG のシミュレータは StoreKit の商品が取れなければモック(`simctl launch` では Scheme の構成が効かない)。Release は常に StoreKit。
    private static var storeMode: ProEntitlementStore.Mode {
        #if DEBUG
        switch ProcessInfo.processInfo.environment["METAMON_MOCK_STORE"] {
        case "1": return .mock
        case "0": return .storeKit
        default:
            #if targetEnvironment(simulator)
            return .storeKitOrMock
            #else
            return .storeKit
            #endif
        }
        #else
        return .storeKit
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
#endif
