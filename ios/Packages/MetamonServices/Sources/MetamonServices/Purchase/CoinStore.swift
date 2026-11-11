// 審査用ビルド(APP_STORE_REVIEW・#450 G1)は App 内課金を持たない。コインのパック(StoreKit)もバイナリから外す。
#if !APP_STORE_REVIEW
import Foundation
import Observation
import StoreKit

/// お店に並べる 1 つのパック。値段は StoreKit の表示価格、枚数はサーバーの設定(#630)。
public struct CoinOffer: Equatable, Sendable, Identifiable {
    public var pack: CoinPack
    /// 地域の通貨で整形済みの価格。nil = モック(App Store に行かない)
    public var displayPrice: String?

    public init(pack: CoinPack, displayPrice: String?) {
        self.pack = pack
        self.displayPrice = displayPrice
    }

    public var id: String { pack.productId }
}

/// コイン(#630)の状態: 公開の設定(フラグ)・残高・パックの購入。
/// 購入は StoreKit 2 の消耗型。検証済みの取引の JWS をサーバーに送り、**サーバーが足してから** `finish()` する
/// (足す前に落ちても `Transaction.unfinished` に残り、次の起動で送り直す。サーバーは transactionId で冪等)。
@MainActor
@Observable
public final class CoinStore {
    public enum Mode: Equatable, Sendable {
        case storeKit
        /// 常にモック(UI テスト)
        case mock
        /// StoreKit で商品が取れなければモック(シミュレータを `simctl launch` で起動した時)
        case storeKitOrMock
    }

    public enum PurchaseOutcome: Equatable, Sendable {
        /// サーバーが足した
        case credited
        /// 買えたがサーバーにまだ届いていない(次の起動で送り直す)
        case awaitingCredit
        /// 保護者の承認待ちなど
        case pending
        case cancelled
    }

    public enum StoreError: Error, Equatable {
        case unverified
        case productNotFound
        case disabled
        /// 購入に結び付ける uid が無い(サインインしていない)
        case notSignedIn
    }

    /// 公開の設定。nil = まだ読んでいない
    public private(set) var config: CoinPlanConfig?
    public private(set) var wallet: CoinWallet?
    public private(set) var isLoadingWallet = false
    public private(set) var walletError = false
    public private(set) var isLoadingProducts = false
    public private(set) var products: [Product] = []
    public private(set) var isMock: Bool

    /// サーバーの口。nil = コインを使わない
    @ObservationIgnored public var service: (any CoinService)?
    /// 購入の `appAccountToken` に入れる uid
    @ObservationIgnored public var accountUID: (@Sendable () -> String?)?
    @ObservationIgnored public let mode: Mode
    @ObservationIgnored private var updatesTask: Task<Void, Never>?
    @ObservationIgnored private var started = false
    /// `clear()` の回数。読む間にサインアウトしたら前のアカウントの残高を書き戻さない
    @ObservationIgnored private var generation = 0

    public init(mode: Mode = .storeKit, service: (any CoinService)? = nil) {
        self.mode = mode
        self.service = service
        isMock = mode == .mock
    }

    /// サーバーがコイン課金を有効にしているか。false の間は UI を一切出さない(フラグ)
    public var isEnabled: Bool { service != nil && config?.isEnabled == true }

    /// 「約 N コイン」。残高の応答の目安を優先し、無ければ公開の設定
    public func estimate(_ op: CoinOperation) -> Int? {
        guard isEnabled else { return nil }
        return wallet?.estimates.coins(for: op) ?? config?.estimates.coins(for: op)
    }

    /// お店に並べるパック(サーバーの順)。StoreKit に無いパックは出さない(モックは全部・値段なし)
    public var offers: [CoinOffer] {
        guard let config, config.isEnabled else { return [] }
        return config.packs.compactMap { pack in
            if isMock { return CoinOffer(pack: pack, displayPrice: nil) }
            guard let product = products.first(where: { $0.id == pack.productId }) else { return nil }
            return CoinOffer(pack: pack, displayPrice: product.displayPrice)
        }
    }

    /// 起動時に 1 回。設定を読み、有効なら消耗型の取引の監視と、足し残しの送り直しを始める。
    public func start() {
        Task { await refreshConfig() }
        guard !started, !isMock else { return }
        started = true
        updatesTask = Task { [weak self] in
            for await update in Transaction.updates {
                guard let self else { return }
                await self.handle(update)
            }
        }
        Task { await creditUnfinished() }
    }

    public func refreshConfig() async {
        guard let service else { return }
        do {
            config = try await service.planConfig()
        } catch {
            print("[Metamon] coins: config failed: \(error)")
            if config == nil { config = .disabled } // 読めない時は出さない(今のまま)
        }
    }

    @discardableResult
    public func refreshWallet() async -> CoinWallet? {
        guard let service, isEnabled else { return nil }
        let gen = generation
        isLoadingWallet = true
        defer { if gen == generation { isLoadingWallet = false } }
        do {
            let fresh = try await service.wallet()
            guard gen == generation else { return nil }
            wallet = fresh
            walletError = false
        } catch {
            guard gen == generation else { return nil }
            walletError = true
            print("[Metamon] coins: wallet failed: \(error)")
        }
        return wallet
    }

    public func loadProducts() async {
        guard let config, config.isEnabled, !isMock else { return }
        let ids = config.packs.map(\.productId)
        guard !ids.isEmpty else { return }
        isLoadingProducts = true
        defer { isLoadingProducts = false }
        do {
            products = try await Product.products(for: ids)
        } catch {
            print("[Metamon] coins: products failed: \(error)")
        }
        if products.isEmpty, mode == .storeKitOrMock {
            isMock = true
            print("[Metamon] coins: no StoreKit products → mock store")
        }
    }

    public func purchase(productID: String) async throws -> PurchaseOutcome {
        guard isEnabled else { throw StoreError.disabled }
        if isMock {
            guard config?.pack(productID) != nil else { throw StoreError.productNotFound }
            let ok = await creditThenFinish(signedTransaction: "mock:\(productID):\(UUID().uuidString)") {}
            return ok ? .credited : .awaitingCredit
        }
        if products.isEmpty { await loadProducts() }
        guard let product = products.first(where: { $0.id == productID }) else { throw StoreError.productNotFound }
        // どの uid の購入か(#314 と同じ)。サーバーはこの uid の取引だけを足すので、uid が無ければ買わせない
        guard let uid = accountUID?(), !uid.isEmpty else { throw StoreError.notSignedIn }
        let options: Set<Product.PurchaseOption> = [.appAccountToken(ProAccountToken.uuid(for: uid))]
        switch try await product.purchase(options: options) {
        case .success(let verification):
            guard case .verified(let transaction) = verification else { throw StoreError.unverified }
            let ok = await creditThenFinish(signedTransaction: verification.jwsRepresentation) { await transaction.finish() }
            return ok ? .credited : .awaitingCredit
        case .pending:
            return .pending
        case .userCancelled:
            return .cancelled
        @unknown default:
            return .cancelled
        }
    }

    /// サーバーに送って足してもらい、成功した時だけ `finish` を呼ぶ。戻り値 = 足せたか。
    /// 失敗した取引は終わらせない(StoreKit が `Transaction.unfinished` に残し、次の起動で送り直す)。
    @discardableResult
    public func creditThenFinish(signedTransaction: String, finish: @escaping @Sendable () async -> Void) async -> Bool {
        guard let service else { return false }
        do {
            let fresh = try await service.creditApplePurchase(signedTransaction: signedTransaction)
            await finish()
            wallet = fresh
            walletError = false
            return true
        } catch {
            print("[Metamon] coins: credit failed (kept unfinished): \(error)")
            return false
        }
    }

    /// 枠を超えた後のコイン払い(既定 OFF)と月の上限。成功したら残高の応答で置き換える
    public func updateOverflow(enabled: Bool, monthlyCap: Int) async throws {
        guard let service else { throw StoreError.disabled }
        wallet = try await service.updateSettings(CoinSettingsUpdate(overflowEnabled: enabled, monthlyCap: monthlyCap))
    }

    /// サインアウト・アカウント削除(別のアカウントの残高を出さない)。設定は公開なので残す
    public func clear() {
        generation += 1
        wallet = nil
        walletError = false
        isLoadingWallet = false
    }

    // MARK: - 取引の監視

    private func handle(_ result: VerificationResult<Transaction>) async {
        guard case .verified(let t) = result, t.productType == .consumable, t.revocationDate == nil else { return }
        await creditThenFinish(signedTransaction: result.jwsRepresentation) { await t.finish() }
    }

    /// 前の起動で足し残した消耗型(サーバーに届く前に落ちた・圏外だった)を送り直す
    private func creditUnfinished() async {
        if config == nil { await refreshConfig() }
        guard isEnabled else { return }
        for await result in Transaction.unfinished {
            await handle(result)
        }
    }
}
#endif
