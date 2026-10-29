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

    private func setMockEntitlement() {
        isPro = mockPurchased
        activeProductID = mockPurchased ? ProProducts.monthly : nil
        expiresAt = mockPurchased ? Calendar.current.date(byAdding: .month, value: 1, to: .now) : nil
        print("[Metamon] IAP entitlement (mock): isPro=\(isPro)")
    }
}
