// 審査用ビルド(APP_STORE_REVIEW・#450 G1)は App 内課金を持たない。StoreKit と商品 ID をバイナリから外す。
#if !APP_STORE_REVIEW
import Foundation
import Observation
import StoreKit

/// 有効な権利かどうか(純粋関数・#77)。取り消し済み・期限切れ・対象外の商品は無効。
public enum ProEntitlement {
    public static func isActive(productID: String, revocationDate: Date?, expirationDate: Date?,
                                now: Date = .now, productIDs: [String] = ProProducts.all) -> Bool {
        guard productIDs.contains(productID), revocationDate == nil else { return false }
        if let expirationDate { return expirationDate > now }
        return true // 買い切り(非消耗)は期限なし
    }
}

/// 課金画面に出す商品(#414)。StoreKit の `Product` からも、モックからも作れる。
public struct ProOffer: Equatable, Sendable {
    public var id: String
    public var displayName: String
    /// 地域の通貨で整形済みの価格(例: ¥500)
    public var displayPrice: String

    public init(id: String, displayName: String, displayPrice: String) {
        self.id = id
        self.displayName = displayName
        self.displayPrice = displayPrice
    }

    /// StoreKit が使えない時(UI テスト・`simctl launch`)の月額。`ios/StoreKit/TailorItPro.storekit` と同じ値
    public static let mockMonthly = ProOffer(id: ProProducts.monthly, displayName: "Tailor It Pro", displayPrice: "¥500")
}

/// StoreKit 2 の薄いラッパー(#77)。商品の取得・購入・復元・権利(`Transaction.currentEntitlements`)の監視。
/// 外部サービス(RevenueCat 等)は使わない。ローカル検証は Scheme の StoreKit Configuration(`ios/StoreKit/TailorItPro.storekit`)。
/// `mode` が mock の時は App Store に触れず、購入は端末内で即成功する(DEBUG の UI テスト・シミュレータ用・#414)。
@MainActor
@Observable
public final class ProEntitlementStore {
    public enum PurchaseOutcome: Equatable, Sendable {
        case purchased
        /// 保護者の承認待ち(Ask to Buy)など。確定すると `Transaction.updates` で届く
        case pending
        case cancelled
    }

    public enum Mode: Equatable, Sendable {
        case storeKit
        /// 常にモック(UI テスト)
        case mock
        /// StoreKit で商品が取れなければモックに切り替える(シミュレータを `simctl launch` で起動した時)
        case storeKitOrMock
    }

    public enum StoreError: Error, Equatable {
        case unverified
        case productNotFound
    }

    public private(set) var products: [Product] = []
    public private(set) var isPro = false
    public private(set) var activeProductID: String?
    /// サブスクの次回更新(期限)。買い切りなら nil
    public private(set) var expiresAt: Date?
    public private(set) var isLoadingProducts = false
    /// 商品が取れなかった(StoreKit Configuration 未設定・圏外など)
    public private(set) var productsError: String?

    /// モックで動いているか(購入・復元が App Store に行かない)
    public private(set) var isMock = false

    /// サーバーが返した Pro(#314)。nil = まだ送っていない / 送れなかった
    public private(set) var serverEntitlements: ServerEntitlements?

    /// 検証済みの取引を送る先(#314)。nil なら送らない(モック・appconfig が無い構成)
    @ObservationIgnored public var server: (any EntitlementClient)?
    /// いまの uid(購入の `appAccountToken`・送信済みの記録の鍵)
    @ObservationIgnored public var accountUID: (@Sendable () -> String?)?
    /// 送った取引(uid + JWS)。同じものを何度も送らない。失敗したものは入れない(次の起動・復元で送り直す)
    @ObservationIgnored private var submitted: Set<String> = []
    /// 送信中の処理(テストで待つ)
    @ObservationIgnored public private(set) var serverSyncTask: Task<Void, Never>?

    public let productIDs: [String]
    public let mode: Mode
    @ObservationIgnored private var updatesTask: Task<Void, Never>?
    @ObservationIgnored private var started = false
    @ObservationIgnored private var mockPurchased = false

    public init(productIDs: [String] = ProProducts.all, mode: Mode = .storeKit) {
        self.productIDs = productIDs
        self.mode = mode
        isMock = mode == .mock
    }

    /// 課金画面の商品(月額)。モックなら固定値
    public var offer: ProOffer? {
        if isMock { return .mockMonthly }
        return monthly.map { ProOffer(id: $0.id, displayName: $0.displayName, displayPrice: $0.displayPrice) }
    }

    /// 起動時に 1 回。`Transaction.updates`(他端末での購入・更新・返金・Ask to Buy の確定)を監視し、現在の権利と商品を読む。
    /// 2 回目以降(サインインし直して画面が作り直された時)は権利だけ読み直す(新しい uid でサーバーに送る・#314)。
    public func start() {
        guard !started else {
            Task { await refreshEntitlements() }
            return
        }
        started = true
        updatesTask = Task { [weak self] in
            for await update in Transaction.updates {
                guard let self else { return }
                // コインのパック(消耗型・#630)は `CoinStore` がサーバーに足してから終わらせる。ここで終わらせない
                if case .verified(let transaction) = update, self.productIDs.contains(transaction.productID) {
                    await transaction.finish()
                }
                await self.refreshEntitlements()
            }
        }
        Task {
            await refreshEntitlements()
            await loadProducts()
        }
    }

    public var monthly: Product? { products.first { $0.id == ProProducts.monthly } ?? products.first }

    public func loadProducts() async {
        if isMock { return }
        isLoadingProducts = true
        defer { isLoadingProducts = false }
        do {
            let fetched = try await Product.products(for: productIDs)
            products = fetched.sorted { $0.price < $1.price }
            productsError = fetched.isEmpty ? "no products" : nil
        } catch {
            productsError = String(describing: error)
        }
        if products.isEmpty, mode == .storeKitOrMock {
            isMock = true
            print("[Metamon] IAP: no StoreKit products → mock store")
        }
        print("[Metamon] IAP products: \(products.map(\.id)) error=\(productsError ?? "-") mock=\(isMock)")
    }

    /// 検証済みの現在の権利から Pro かどうかを決め直す。
    public func refreshEntitlements() async {
        if isMock {
            setMockEntitlement()
            return
        }
        var active: (id: String, expires: Date?)?
        var signed: [String] = []
        for await result in Transaction.currentEntitlements {
            guard case .verified(let t) = result, productIDs.contains(t.productID) else { continue }
            // Xcode の StoreKit 構成の取引は Apple の証明書で署名されていないので、サーバーは受け取らない
            if t.environment != .xcode { signed.append(result.jwsRepresentation) }
            guard ProEntitlement.isActive(productID: t.productID, revocationDate: t.revocationDate,
                                          expirationDate: t.expirationDate, productIDs: productIDs) else { continue }
            active = (t.productID, t.expirationDate)
        }
        sendToServer(signed)
        isPro = active != nil
        activeProductID = active?.id
        expiresAt = active?.expires
        print("[Metamon] IAP entitlement: isPro=\(isPro) product=\(activeProductID ?? "-")")
    }

    public func purchase(_ product: Product) async throws -> PurchaseOutcome {
        var options: Set<Product.PurchaseOption> = []
        if let uid = accountUID?(), !uid.isEmpty {
            options.insert(.appAccountToken(ProAccountToken.uuid(for: uid))) // #314: どの uid の購入か
        }
        switch try await product.purchase(options: options) {
        case .success(let verification):
            guard case .verified(let transaction) = verification else { throw StoreError.unverified }
            await transaction.finish()
            await refreshEntitlements()
            return .purchased
        case .pending:
            return .pending
        case .userCancelled:
            return .cancelled
        @unknown default:
            return .cancelled
        }
    }

    public func purchase(productID: String) async throws -> PurchaseOutcome {
        if products.isEmpty { await loadProducts() }
        if isMock {
            guard productIDs.contains(productID) else { throw StoreError.productNotFound }
            mockPurchased = true
            setMockEntitlement()
            return .purchased
        }
        guard let product = products.first(where: { $0.id == productID }) else { throw StoreError.productNotFound }
        return try await purchase(product)
    }

    /// 購入の復元。App Store と同期してから権利を読み直す(Apple ID のサインインを求められることがある)。
    public func restore() async throws {
        if !isMock { try await AppStore.sync() }
        await refreshEntitlements()
    }

    /// 課金画面の「Pro にする」(月額)。
    public func purchaseOffer() async throws -> PurchaseOutcome {
        try await purchase(productID: offer?.id ?? ProProducts.monthly)
    }

    /// 検証済みの取引(JWS)をサーバーに送る(#314)。購入・復元・起動・`Transaction.updates` のあと `refreshEntitlements` から呼ばれる。
    /// 待たない(購入の完了をネットワークで遅らせない)。失敗はログだけで、次の機会に送り直す。
    func sendToServer(_ signedTransactions: [String]) {
        guard let server, !signedTransactions.isEmpty else { return }
        let uid = accountUID?() ?? ""
        let pending = signedTransactions.filter { !submitted.contains(uid + "|" + $0) }
        guard !pending.isEmpty else { return }
        let previous = serverSyncTask
        serverSyncTask = Task { [weak self] in
            await previous?.value
            for jws in pending {
                do {
                    let result = try await server.submitAppleTransaction(jws)
                    self?.submitted.insert(uid + "|" + jws)
                    self?.serverEntitlements = result
                    print("[Metamon] IAP server entitlement: isPro=\(result.isPro)")
                } catch {
                    print("[Metamon] IAP server sync failed: \(error)")
                }
            }
        }
    }

    private func setMockEntitlement() {
        isPro = mockPurchased
        activeProductID = mockPurchased ? ProProducts.monthly : nil
        expiresAt = mockPurchased ? Calendar.current.date(byAdding: .month, value: 1, to: .now) : nil
        print("[Metamon] IAP entitlement (mock): isPro=\(isPro)")
    }
}
#endif
