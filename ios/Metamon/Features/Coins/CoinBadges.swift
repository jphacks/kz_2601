#if !APP_STORE_REVIEW // 審査用ビルドはコインを持たない(#450)
import SwiftUI
import MetamonServices

/// 読み込み中の細い棒(データ無しと区別するため、読み込み中は必ずこれを出す)。
struct CoinSkeleton: View {
    var width: CGFloat = 80
    var height: CGFloat = 14
    @State private var pulse = false

    var body: some View {
        RoundedRectangle(cornerRadius: height / 2, style: .continuous)
            .fill(Theme.Colors.surfaceElevated)
            .frame(width: width, height: height)
            .opacity(pulse ? 0.45 : 1)
            .animation(.easeInOut(duration: 0.9).repeatForever(autoreverses: true), value: pulse)
            .onAppear { pulse = true }
            .accessibilityLabel("読み込み中")
    }
}

/// コインの印(全画面で同じ記号)。
enum CoinSymbol {
    static let name = "c.circle.fill"
}

/// コインの残高のチップ(設定・課金画面)。押すとお店。フラグが OFF なら何も出さない。
struct CoinBalanceChip: View {
    var accessibilityID = "coins.balance"
    private var gate: CoinGate { CoinGate.shared }

    var body: some View {
        if gate.isEnabled {
            Button { gate.openShop() } label: { label }
                .buttonStyle(.plain)
                .accessibilityIdentifier(accessibilityID)
                .task { if gate.store.wallet == nil { await gate.store.refreshWallet() } }
        }
    }

    @ViewBuilder
    private var label: some View {
        HStack(spacing: Theme.Spacing.xs) {
            Image(systemName: CoinSymbol.name)
                .foregroundStyle(Theme.brandGradient)
            if let wallet = gate.store.wallet {
                Text(CoinCopy.coins(wallet.balance.available))
                    .font(.subheadline.weight(.semibold))
                    .monospacedDigit()
                    .foregroundStyle(Theme.Colors.brandText)
            } else if gate.store.walletError {
                Text(verbatim: "—").foregroundStyle(.secondary)
            } else {
                CoinSkeleton(width: 64)
            }
            Image(systemName: "plus.circle.fill")
                .font(.footnote)
                .foregroundStyle(Theme.Colors.brandText)
        }
        .padding(.horizontal, Theme.Spacing.md)
        .padding(.vertical, Theme.Spacing.sm)
        .background(Theme.Colors.brandA.opacity(0.12), in: Capsule())
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(CoinCopy.balanceTitle)
        .accessibilityValue(gate.store.wallet.map { CoinCopy.coins($0.balance.available) } ?? "")
        .accessibilityHint(Text("コインを買う"))
    }
}

/// 送る前の「約 N コイン」(作る・直す)。目安が無い・フラグが OFF なら出さない。
struct CoinEstimateLine: View {
    let operation: CoinOperation
    private var gate: CoinGate { CoinGate.shared }

    var body: some View {
        if let n = gate.store.estimate(operation) {
            let balance = gate.store.wallet?.balance.available
            let short = balance.map { $0 < n } ?? false
            Button { gate.openShop() } label: {
                HStack(spacing: Theme.Spacing.xs) {
                    Image(systemName: short ? "exclamationmark.circle.fill" : CoinSymbol.name)
                    Text(CoinCopy.estimate(n)).monospacedDigit()
                    if let balance {
                        Text(verbatim: "·").foregroundStyle(.tertiary)
                        Text(L10n.tr("残り \(balance)")).monospacedDigit()
                    }
                }
                .font(.caption.weight(short ? .semibold : .regular))
                .foregroundStyle(short ? AnyShapeStyle(Theme.Colors.warning) : AnyShapeStyle(.secondary))
            }
            .buttonStyle(.plain)
            .frame(maxWidth: .infinity, alignment: .trailing)
            .padding(.bottom, Theme.Spacing.xs)
            .accessibilityElement(children: .ignore)
            .accessibilityLabel(CoinCopy.estimateAccessibility(n, balance: balance))
            .accessibilityAddTraits(.isButton)
            .accessibilityIdentifier("voice.coinEstimate")
            .task { if gate.store.wallet == nil { await gate.store.refreshWallet() } }
        }
    }
}
#endif
