#if !APP_STORE_REVIEW // 審査用ビルドは App 内課金を持たない(#450)
import SwiftUI
import MetamonRenderer
import MetamonServices

/// コインのお店(#630)。パックは StoreKit の値段 + サーバーの枚数 + おまけの印。
/// 匿名のアカウントでは買えない(消耗型は復元されず、残高はサーバーの uid に持つため)。
struct CoinShopSheet: View {
    /// 開いた理由(足りなくて止まった時は先に説明する)
    var reason: CoinDenial?
    @Environment(\.dismiss) private var dismiss
    @Environment(\.colorScheme) private var scheme
    @Environment(AuthSession.self) private var session
    @State private var busyID: String?
    @State private var message: String?

    private var gate: CoinGate { CoinGate.shared }
    private var store: CoinStore { gate.store }
    private var isAnonymous: Bool { session.user?.isAnonymous ?? true }

    var body: some View {
        NavigationStack {
            ZStack {
                Theme.backdrop(scheme)
                ScrollView {
                    VStack(spacing: Theme.Spacing.lg) {
                        header
                        if let reason { reasonCard(reason) }
                        if isAnonymous { signInNote }
                        packs
                        if let message {
                            Text(message)
                                .font(.footnote)
                                .foregroundStyle(.secondary)
                                .multilineTextAlignment(.center)
                                .accessibilityIdentifier("coins.shop.message")
                        }
                        Button { gate.openPlans() } label: {
                            Label("プランとコインを比べる", systemImage: "square.split.2x1")
                        }
                        .font(.subheadline.weight(.medium))
                        .tint(Theme.Colors.brandText)
                        .accessibilityIdentifier("coins.shop.plans")
                        finePrint
                    }
                    .padding(.horizontal, Theme.Spacing.lg)
                    .padding(.vertical, Theme.Spacing.xl)
                }
            }
            .navigationTitle(Text("コイン"))
            .navigationBarTitleDisplayMode(.inline)
            .toolbar { SheetCloseToolbarItem(identifier: "coins.shop.close") { dismiss() } }
        }
        .presentationDetents([.large])
        .presentationDragIndicator(.visible)
        .task {
            await store.refreshWallet()
            if store.offers.isEmpty { await store.loadProducts() }
        }
    }

    private var header: some View {
        VStack(spacing: Theme.Spacing.sm) {
            Image(systemName: CoinSymbol.name)
                .font(.system(size: 44, weight: .semibold))
                .foregroundStyle(Theme.brandGradient)
                .accessibilityHidden(true)
            if let wallet = store.wallet {
                Text(CoinCopy.coins(wallet.balance.total))
                    .font(.system(.largeTitle, design: .rounded).weight(.bold))
                    .monospacedDigit()
                    .accessibilityIdentifier("coins.shop.balance")
                balanceBreakdown(wallet.balance)
            } else if store.walletError {
                Text("残高を読み込めませんでした")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            } else {
                CoinSkeleton(width: 140, height: 34)
                CoinSkeleton(width: 180, height: 12)
            }
            Text("AI で作る・直す・アプリの中の AI は、使うたびにコインを使います。")
                .font(.subheadline)
                .foregroundStyle(.secondary)
                .multilineTextAlignment(.center)
                .fixedSize(horizontal: false, vertical: true)
        }
    }

    /// 3.1.1: 買った分(期限なし)と Pro の毎月の分を分ける
    @ViewBuilder
    private func balanceBreakdown(_ b: CoinBalance) -> some View {
        VStack(spacing: Theme.Spacing.xs) {
            Text(L10n.tr("\(CoinCopy.purchasedTitle): \(b.purchased)"))
            if b.grant > 0 { Text(L10n.tr("\(CoinCopy.grantTitle(until: b.grantPeriodEnd)): \(b.grant)")) }
        }
        .font(.caption)
        .foregroundStyle(.secondary)
        .monospacedDigit()
    }

    private func reasonCard(_ d: CoinDenial) -> some View {
        VStack(alignment: .leading, spacing: Theme.Spacing.xs) {
            Label(CoinCopy.denialTitle(d.kind), systemImage: "exclamationmark.circle.fill")
                .font(.headline)
                .foregroundStyle(Theme.Colors.warning)
            Text(CoinCopy.denialBody(d))
                .font(.subheadline)
                .fixedSize(horizontal: false, vertical: true)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .metamonCard()
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("coins.shop.reason")
    }

    private var signInNote: some View {
        Text("コインはアカウントに保存します。買う前に、設定の「アカウント」から Apple でサインインしてください。")
            .font(.footnote)
            .foregroundStyle(.secondary)
            .frame(maxWidth: .infinity, alignment: .leading)
            .metamonCard()
            .accessibilityIdentifier("coins.shop.signIn")
    }

    @ViewBuilder
    private var packs: some View {
        let offers = store.offers
        VStack(spacing: Theme.Spacing.md) {
            if offers.isEmpty {
                if store.isLoadingProducts || store.config == nil {
                    ForEach(0..<3, id: \.self) { _ in packSkeleton }
                } else {
                    Text("コインのパックを取得できませんでした。通信状態を確認してください。")
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                        .accessibilityIdentifier("coins.shop.empty")
                }
            } else {
                ForEach(offers) { offer in packRow(offer) }
            }
        }
    }

    private var packSkeleton: some View {
        HStack {
            VStack(alignment: .leading, spacing: Theme.Spacing.xs) {
                CoinSkeleton(width: 110, height: 18)
                CoinSkeleton(width: 70, height: 10)
            }
            Spacer()
            CoinSkeleton(width: 72, height: 32)
        }
        .metamonCard()
    }

    private func packRow(_ offer: CoinOffer) -> some View {
        HStack(alignment: .center, spacing: Theme.Spacing.md) {
            Image(systemName: CoinSymbol.name)
                .font(.title2)
                .foregroundStyle(Theme.brandGradient)
                .accessibilityHidden(true)
            VStack(alignment: .leading, spacing: Theme.Spacing.xs) {
                Text(CoinCopy.coins(offer.pack.totalCoins))
                    .font(.system(.title3, design: .rounded).weight(.bold))
                    .monospacedDigit()
                if offer.pack.bonusCoins > 0 {
                    Text(L10n.tr("おまけ +\(offer.pack.bonusCoins)"))
                        .font(.caption2.weight(.bold))
                        .foregroundStyle(.white)
                        .padding(.horizontal, Theme.Spacing.sm)
                        .padding(.vertical, 2)
                        .background(Theme.brandGradient, in: Capsule())
                        .accessibilityIdentifier("coins.shop.bonus.\(offer.id)")
                }
            }
            Spacer()
            Button {
                Task { await buy(offer) }
            } label: {
                ZStack {
                    if busyID == offer.id { ProgressView().tint(.white) }
                    Text(offer.displayPrice ?? L10n.tr("テスト"))
                        .font(.headline)
                        .monospacedDigit()
                        .opacity(busyID == offer.id ? 0 : 1)
                }
                .padding(.horizontal, Theme.Spacing.md)
                .padding(.vertical, Theme.Spacing.sm)
                .foregroundStyle(.white)
                .background(Theme.brandGradient, in: Capsule())
            }
            .disabled(busyID != nil || isAnonymous)
            .accessibilityLabel(Text(L10n.tr("\(CoinCopy.coins(offer.pack.totalCoins))を買う")))
            .accessibilityValue(offer.displayPrice ?? "")
            .accessibilityIdentifier("coins.shop.buy.\(offer.id)")
        }
        .metamonCard()
    }

    private var finePrint: some View {
        Text("買ったコインに期限はありません。Pro の毎月のコインはその月だけ使え、繰り越しません。")
            .font(.caption2)
            .foregroundStyle(.secondary)
            .multilineTextAlignment(.center)
    }

    private func buy(_ offer: CoinOffer) async {
        busyID = offer.id
        defer { busyID = nil }
        do {
            switch try await store.purchase(productID: offer.id) {
            case .credited: message = L10n.tr("\(CoinCopy.coins(offer.pack.totalCoins))を足しました。")
            case .awaitingCredit: message = L10n.tr("購入できました。コインの反映を待っています(次に開いた時に反映されます)。")
            case .pending: message = L10n.tr("承認待ちです。承認されると自動でコインが足されます。")
            case .cancelled: break
            }
        } catch {
            message = L10n.tr("購入できませんでした。時間をおいて試してください。")
            print("[Metamon] coins: purchase failed: \(error)")
        }
    }
}
#endif
