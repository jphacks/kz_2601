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

/// StoreKit 2 の薄いラッパー(#77)。商品の取得・購入・復元・権利(`Transaction.currentEntitlements`)の監視。
/// 外部サービス(RevenueCat 等)は使わない。ローカル検証は Scheme の StoreKit Configuration(`ios/StoreKit/TailorItPro.storekit`)。
@MainActor
@Observable
public final class ProEntitlementStore {
    public enum PurchaseOutcome: Equatable, Sendable {
        case purchased
        /// 保護者の承認待ち(Ask to Buy)など。確定すると `Transaction.updates` で届く
        case pending
        case cancelled
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

    public let productIDs: [String]
    @ObservationIgnored private var updatesTask: Task<Void, Never>?
    @ObservationIgnored private var started = false

    public init(productIDs: [String] = ProProducts.all) {
        self.productIDs = productIDs
    }

    /// 起動時に 1 回。`Transaction.updates`(他端末での購入・更新・返金・Ask to Buy の確定)を監視し、現在の権利と商品を読む。
    public func start() {
        guard !started else { return }
        started = true
        updatesTask = Task { [weak self] in
            for await update in Transaction.updates {
                guard let self else { return }
                if case .verified(let transaction) = update { await transaction.finish() }
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
        isLoadingProducts = true
        defer { isLoadingProducts = false }
        do {
            let fetched = try await Product.products(for: productIDs)
            products = fetched.sorted { $0.price < $1.price }
            productsError = fetched.isEmpty ? "no products" : nil
        } catch {
            productsError = String(describing: error)
        }
        print("[Metamon] IAP products: \(products.map(\.id)) error=\(productsError ?? "-")")
    }

    /// 検証済みの現在の権利から Pro かどうかを決め直す。
    public func refreshEntitlements() async {
        var active: (id: String, expires: Date?)?
        for await result in Transaction.currentEntitlements {
            guard case .verified(let t) = result,
                  ProEntitlement.isActive(productID: t.productID, revocationDate: t.revocationDate,
                                          expirationDate: t.expirationDate, productIDs: productIDs) else { continue }
            active = (t.productID, t.expirationDate)
        }
        isPro = active != nil
        activeProductID = active?.id
        expiresAt = active?.expires
        print("[Metamon] IAP entitlement: isPro=\(isPro) product=\(activeProductID ?? "-")")
    }

    public func purchase(_ product: Product) async throws -> PurchaseOutcome {
        switch try await product.purchase() {
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
        guard let product = products.first(where: { $0.id == productID }) else { throw StoreError.productNotFound }
        return try await purchase(product)
    }

    /// 購入の復元。App Store と同期してから権利を読み直す(Apple ID のサインインを求められることがある)。
    public func restore() async throws {
        try await AppStore.sync()
        await refreshEntitlements()
    }
}
