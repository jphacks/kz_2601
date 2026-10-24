import SwiftUI
import StoreKit
import MetamonServices

/// 「Tailor It Pro」の購入シート(#77)。上限に達した時(`reason == .limitReached`)と設定の「Pro にする」から開く。
struct PaywallSheet: View {
    enum Reason { case limitReached, settings }

    var reason: Reason = .settings
    @Environment(\.dismiss) private var dismiss
    @Environment(\.colorScheme) private var scheme
    @State private var busy = false
    @State private var message: String?
    @State private var justPurchased = false

    private var gate: ProGate { ProGate.shared }
    private var store: ProEntitlementStore { gate.store }

    var body: some View {
        NavigationStack {
            ZStack {
                Theme.backdrop(scheme)
                ScrollView {
                    VStack(spacing: Theme.Spacing.lg) {
                        header
                        benefits
                        if store.isPro { proBadge } else { purchaseArea }
                        if let message {
                            Text(message)
                                .font(.footnote)
                                .foregroundStyle(.secondary)
                                .multilineTextAlignment(.center)
                                .accessibilityIdentifier("paywall.message")
                        }
                        finePrint
                    }
                    .padding(.horizontal, Theme.Spacing.lg)
                    .padding(.vertical, Theme.Spacing.xl)
                }
            }
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button("閉じる") { dismiss() }
                        .accessibilityIdentifier("paywall.close")
                }
            }
        }
        .presentationDetents([.large])
        .presentationDragIndicator(.visible)
        .task { if store.products.isEmpty { await store.loadProducts() } }
        .onChange(of: store.isPro) { _, isPro in
            guard isPro else { return }
            justPurchased = true
            Task {
                try? await Task.sleep(for: .seconds(1.4))
                dismiss()
            }
        }
    }

    private var header: some View {
        VStack(spacing: Theme.Spacing.md) {
            BrandMark(size: 72)
            Text(verbatim: "Tailor It Pro")
                .font(.largeTitle.weight(.bold))
                .brandText()
            Text(reason == .limitReached
                 ? "今日の無料作成(\(gate.quota.dailyFreeLimit) 回)を使い切りました"
                 : "新しいアプリを、いくつでも")
                .font(.headline)
                .multilineTextAlignment(.center)
                .accessibilityIdentifier("paywall.headline")
        }
    }

    private var benefits: some View {
        VStack(alignment: .leading, spacing: Theme.Spacing.md) {
            benefit("infinity", "毎日、新しいアプリを無制限に作れます")
            benefit("waveform", "作ったアプリへの「ここを変えて」はこれまでどおり無料")
            benefit("arrow.uturn.backward.circle", "いつでも解約できます(App Store の設定から)")
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .metamonCard()
    }

    private func benefit(_ symbol: String, _ text: LocalizedStringKey) -> some View {
        HStack(alignment: .firstTextBaseline, spacing: Theme.Spacing.md) {
            Image(systemName: symbol)
                .font(.body.weight(.semibold))
                .foregroundStyle(Theme.brandGradient)
                .frame(width: 24)
            Text(text).font(.body)
        }
    }

    private var purchaseArea: some View {
        VStack(spacing: Theme.Spacing.md) {
            Button {
                Task { await buy() }
            } label: {
                HStack(spacing: Theme.Spacing.sm) {
                    if busy { ProgressView().tint(.white) }
                    Text(priceLabel).font(.headline)
                }
                .frame(maxWidth: .infinity)
                .padding(.vertical, Theme.Spacing.md)
                .foregroundStyle(.white)
                .background(Theme.brandGradient, in: Capsule())
                .shadow(color: Theme.Colors.brandA.opacity(0.3), radius: 12, y: 6)
            }
            .disabled(busy || store.monthly == nil)
            .accessibilityIdentifier("paywall.purchase")

            Button("購入を復元") { Task { await restore() } }
                .font(.subheadline.weight(.medium))
                .foregroundStyle(Theme.Colors.brandText)
                .disabled(busy)
                .accessibilityIdentifier("paywall.restore")

            if store.monthly == nil, !store.isLoadingProducts {
                Text("価格を取得できませんでした。通信状態を確認してください。")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }
        }
    }

    private var proBadge: some View {
        Label(justPurchased ? "Pro になりました" : "Pro をご利用中です", systemImage: "checkmark.seal.fill")
            .font(.headline)
            .foregroundStyle(Theme.Colors.brandText)
            .padding(.vertical, Theme.Spacing.md)
            .frame(maxWidth: .infinity)
            .metamonCard()
            .accessibilityIdentifier("paywall.pro")
    }

    private var finePrint: some View {
        Text("月額の自動更新サブスクリプションです。期間終了の 24 時間前までに解約しない限り自動で更新されます。")
            .font(.caption2)
            .foregroundStyle(.secondary)
            .multilineTextAlignment(.center)
    }

    private var priceLabel: String {
        guard let product = store.monthly else { return L10n.tr("Pro にする") }
        return L10n.tr("Pro にする(\(product.displayPrice) / 月)")
    }

    private func buy() async {
        guard let product = store.monthly else { return }
        busy = true
        defer { busy = false }
        do {
            switch try await store.purchase(product) {
            case .purchased: message = nil
            case .pending: message = L10n.tr("承認待ちです。承認されると自動で Pro になります。")
            case .cancelled: break
            }
        } catch {
            message = L10n.tr("購入できませんでした。時間をおいて試してください。")
            print("[Metamon] IAP purchase failed: \(error)")
        }
    }

    private func restore() async {
        busy = true
        defer { busy = false }
        do {
            try await store.restore()
            if !store.isPro { message = L10n.tr("復元できる購入は見つかりませんでした。") }
        } catch {
            message = L10n.tr("復元できませんでした。時間をおいて試してください。")
            print("[Metamon] IAP restore failed: \(error)")
        }
    }
}

/// 上限シートの present 元 + 起動時のストア開始。RootView の外側(MetamonApp)に 1 つだけ付ける。
struct PaywallHost: ViewModifier {
    func body(content: Content) -> some View {
        @Bindable var gate = ProGate.shared
        return content
            .sheet(isPresented: $gate.paywallPresented) { PaywallSheet(reason: .limitReached) }
            .task { ProGate.shared.start() }
    }
}

#Preview {
    PaywallSheet(reason: .limitReached)
}
