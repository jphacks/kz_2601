// 審査用ビルド(APP_STORE_REVIEW・#450 G1)は App 内課金を持たない。コインの画面も外す(docs/ios/review-build.md)。
#if !APP_STORE_REVIEW
import Foundation
import Observation
import MetamonServices

/// コイン(#630)の司令塔: 残高とパックの `CoinStore`、お店・断りの案内・コインの設定の表示状態。
/// サーバーがコイン課金を有効にしていない間(`isEnabled == false`)は、どの画面にも何も出さない。
@MainActor
@Observable
final class CoinGate {
    static let shared = CoinGate()

    /// 出すシート
    enum Sheet: Equatable {
        case shop(CoinDenial?)
        case plans
        case settings(CoinDenial)
    }

    /// 表示の要求。`presenter` = どの `CoinSheetPresenter` が出すか(いちばん手前のシートの上に重ねる)
    struct Request: Identifiable, Equatable {
        let id = UUID()
        var presenter: UUID
        var sheet: Sheet
    }

    let store: CoinStore
    private(set) var request: Request?
    /// 今見えている presenter(後ろほど手前)。シートの上でシートを出せないので、いちばん手前から出す
    @ObservationIgnored private var presenters: [UUID] = []
    @ObservationIgnored private var installed = false

    init(store: CoinStore? = nil) {
        self.store = store ?? CoinStore(mode: Self.storeMode)
    }

    var isEnabled: Bool { store.isEnabled }

    /// `CoinHost` の task から 1 回。口(live は appconfig・DEBUG の `METAMON_COINS=1` はモック)を入れて始める
    func install(service: (any CoinService)?, accountUID: @escaping @Sendable () -> String?) {
        guard !installed else {
            Task { await store.refreshConfig() }
            return
        }
        installed = true
        store.service = Self.debugService ?? service
        store.accountUID = accountUID
        store.start()
    }

    func openShop(_ reason: CoinDenial? = nil) { show(.shop(reason)) }

    func openPlans() { show(.plans) }

    func show(_ sheet: Sheet) {
        guard isEnabled, let top = presenters.last else { return }
        request = Request(presenter: top, sheet: sheet)
    }

    func dismiss() { request = nil }

    func presenterAppeared(_ id: UUID) {
        presenters.removeAll { $0 == id }
        presenters.append(id)
    }

    func presenterDisappeared(_ id: UUID) {
        presenters.removeAll { $0 == id }
        if request?.presenter == id { request = nil }
    }

    /// 402 のコインの断り。足りない(自分が払う)→ お店、枠・上限 → 説明 + コインの設定。true = 引き受けた
    @discardableResult
    func handle(_ denial: CoinDenial) -> Bool {
        print("[Metamon] coins: denied \(denial.kind.rawValue) required=\(denial.required ?? -1) balance=\(denial.balance ?? -1) payer=\(denial.payer ?? "-")")
        Task { await store.refreshWallet() }
        guard isEnabled, !presenters.isEmpty else { return false }
        switch CoinDenialRoute.of(denial) {
        case .shop: show(.shop(denial))
        case .settings: show(.settings(denial))
        case .explainOnly: return false // 作成者のコイン: 呼び出し側の文言(トースト・カード)に任せる
        }
        return true
    }

    func clear() {
        request = nil
        store.clear()
    }

    private static var storeMode: CoinStore.Mode {
        #if DEBUG || METAMON_STG
        switch DebugPlanKnobs.string(DebugPlanKnobs.mockStore) {
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

    /// DEBUG・stg のみ: `METAMON_COINS=1`(開発用メニューでも・#2080)でサーバー無しにコインの画面を確かめる(端末内のモック・仮の数)。
    /// 残高は `METAMON_COINS_BALANCE`、無効は `METAMON_COINS_DISABLED=1`
    static var debugService: (any CoinService)? {
        #if DEBUG || METAMON_STG
        if DebugPlanKnobs.coinsMocked { return DebugPlanKnobs.coinService() }
        #endif
        return nil
    }
}

/// 断りをどこへ案内するか(純粋関数)。
enum CoinDenialRoute: Equatable {
    case shop
    case settings
    /// 共有アプリの作成者のコイン・枠。参加者が買っても直らないので、お店を出さず説明だけ
    case explainOnly

    static func of(_ denial: CoinDenial) -> CoinDenialRoute {
        guard denial.payerIsSelf else { return .explainOnly }
        switch denial.kind {
        case .coinsInsufficient: return .shop
        case .allowanceExhausted, .overflowCapReached: return .settings
        }
    }
}
#endif
