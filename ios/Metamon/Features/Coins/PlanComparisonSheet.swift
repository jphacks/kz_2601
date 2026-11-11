#if !APP_STORE_REVIEW // 審査用ビルドは App 内課金を持たない(#450)
import SwiftUI
import MetamonRenderer
import MetamonServices

/// プランを比べる(#630): 無料 / Pro / コインの追加。数はサーバーの公開の設定から(無い数は行ごと出さない)。
struct PlanComparisonSheet: View {
    @Environment(\.dismiss) private var dismiss
    @Environment(\.colorScheme) private var scheme

    private var gate: CoinGate { CoinGate.shared }
    private var config: CoinPlanConfig? { gate.store.config }

    var body: some View {
        NavigationStack {
            ZStack {
                Theme.backdrop(scheme)
                ScrollView {
                    VStack(spacing: Theme.Spacing.lg) {
                        Text("AI で作る・直すは、どのプランでも使うたびにコインを使います。")
                            .font(.subheadline)
                            .foregroundStyle(.secondary)
                            .multilineTextAlignment(.center)
                            .fixedSize(horizontal: false, vertical: true)
                        if let config {
                            freeCard(config)
                            proCard(config)
                            coinsCard(config)
                        } else {
                            ForEach(0..<3, id: \.self) { _ in cardSkeleton }
                        }
                    }
                    .padding(.horizontal, Theme.Spacing.lg)
                    .padding(.vertical, Theme.Spacing.xl)
                }
            }
            .navigationTitle(Text("プランを比べる"))
            .navigationBarTitleDisplayMode(.inline)
            .toolbar { SheetCloseToolbarItem(identifier: "coins.plans.close") { dismiss() } }
        }
        .presentationDetents([.large])
        .presentationDragIndicator(.visible)
        .task {
            if config == nil { await gate.store.refreshConfig() }
            if gate.store.offers.isEmpty { await gate.store.loadProducts() }
        }
    }

    // MARK: - カード

    private func freeCard(_ c: CoinPlanConfig) -> some View {
        planCard(title: L10n.tr("無料"), badge: nil, id: "free") {
            row("person.fill", L10n.tr("自分だけのアプリと記録"))
            row(CoinSymbol.name, L10n.tr("AI はコインで使えます"))
            numbers(c.free)
        }
    }

    private func proCard(_ c: CoinPlanConfig) -> some View {
        planCard(title: "Tailor It Pro", badge: ProGate.shared.store.offer.map { L10n.tr("\($0.displayPrice) / 月") }, id: "pro") {
            row("person.2.fill", L10n.tr("共有・オンラインの機能"))
            row("server.rack", L10n.tr("自動の動き・通知などのサーバーの枠(月)"))
            numbers(c.pro)
            row("arrow.uturn.backward.circle", L10n.tr("いつでも解約できます(App Store の設定から)"))
        }
    }

    private func coinsCard(_ c: CoinPlanConfig) -> some View {
        planCard(title: L10n.tr("コインの追加"), badge: nil, id: "coins") {
            row(CoinSymbol.name, L10n.tr("要る分だけ足せます。買ったコインに期限はありません"))
            ForEach(gate.store.offers) { offer in
                numberRow(CoinCopy.coins(offer.pack.totalCoins),
                          value: offer.displayPrice ?? (offer.pack.bonusCoins > 0 ? L10n.tr("おまけ +\(offer.pack.bonusCoins)") : ""))
            }
            row("gauge.with.needle", overflowLine(c))
            Button { gate.openShop() } label: {
                Text("コインを買う")
                    .font(.headline)
                    .frame(maxWidth: .infinity)
                    .padding(.vertical, Theme.Spacing.sm)
                    .foregroundStyle(.white)
                    .background(Theme.brandGradient, in: Capsule())
            }
            .padding(.top, Theme.Spacing.xs)
            .accessibilityIdentifier("coins.plans.buy")
        }
    }

    private func overflowLine(_ c: CoinPlanConfig) -> String {
        if let coins = c.overflowBlockCoins, let runs = c.overflowBlockRuns {
            return L10n.tr("Pro の枠を超えたら、ON にした時だけコインで続けられます(\(runs) 回ごとに \(coins) コイン・月の上限は自分で決めます)")
        }
        return L10n.tr("Pro の枠を超えたら、ON にした時だけコインで続けられます(月の上限は自分で決めます)")
    }

    // MARK: - 部品

    @ViewBuilder
    private func numbers(_ n: CoinPlanNumbers) -> some View {
        if let monthly = n.monthlyCoins, monthly > 0 {
            numberRow(L10n.tr("毎月のコイン(繰り越しません)"), value: CoinCopy.coins(monthly))
        }
        if let once = n.oneTimeCoins, once > 0 {
            numberRow(CoinCopy.initialGrantTitle, value: CoinCopy.coins(once))
        }
        ForEach(CoinCopy.orderedAllowanceKeys(n.allowance.keys), id: \.self) { key in
            if let name = CoinCopy.allowanceName(key), let limit = n.allowance[key] {
                numberRow(name, value: CoinCopy.perMonth(limit))
            }
        }
    }

    private func planCard<Content: View>(title: String, badge: String?, id: String, @ViewBuilder content: () -> Content) -> some View {
        VStack(alignment: .leading, spacing: Theme.Spacing.md) {
            HStack(alignment: .firstTextBaseline) {
                Text(verbatim: title).font(.title3.weight(.bold))
                Spacer()
                if let badge {
                    Text(verbatim: badge)
                        .font(.headline)
                        .monospacedDigit()
                        .foregroundStyle(Theme.Colors.brandText)
                }
            }
            content()
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .metamonCard()
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("coins.plans.\(id)")
    }

    private func row(_ symbol: String, _ text: String) -> some View {
        HStack(alignment: .firstTextBaseline, spacing: Theme.Spacing.md) {
            Image(systemName: symbol)
                .font(.body.weight(.semibold))
                .foregroundStyle(Theme.brandGradient)
                .frame(width: 24)
                .accessibilityHidden(true)
            Text(verbatim: text)
                .font(.body)
                .fixedSize(horizontal: false, vertical: true)
        }
    }

    private func numberRow(_ title: String, value: String) -> some View {
        HStack(alignment: .firstTextBaseline) {
            Text(verbatim: title).font(.subheadline).foregroundStyle(.secondary)
            Spacer()
            Text(verbatim: value).font(.subheadline.weight(.semibold)).monospacedDigit()
        }
        .padding(.leading, 24 + Theme.Spacing.md)
        .accessibilityElement(children: .combine)
    }

    private var cardSkeleton: some View {
        VStack(alignment: .leading, spacing: Theme.Spacing.sm) {
            CoinSkeleton(width: 90, height: 20)
            CoinSkeleton(width: 220, height: 12)
            CoinSkeleton(width: 180, height: 12)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .metamonCard()
    }
}
#endif
