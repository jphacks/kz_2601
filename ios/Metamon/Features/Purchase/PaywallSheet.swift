// 審査用ビルド(APP_STORE_REVIEW・#450 G1)は App 内課金を持たない。
#if !APP_STORE_REVIEW
import SwiftUI
import StoreKit
import MetamonServices

/// 「Tailor It Pro」の課金画面(#77 / #414)。何ができるようになるか・価格・購入・復元・規約とプライバシー(3.1.2)。
/// 開く入口: 設定の「Pro にする」/ 無料作成の上限 / 有料機能(共有など)を使うアプリの生成・修正 / 共有アプリへの参加。
struct PaywallSheet: View {
    var reason: PaywallReason = .settings
    /// 閉じた(買わなかった)時にどうなるか。有料機能の入口だけ出す
    var closeNote: String?
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
                        if case .paidFeature(let features) = reason { featureReasons(features) }
                        // 購入まわりを先に(最初の画面に価格と購入ボタンが入るように)
                        if store.isPro { proBadge } else { purchaseArea }
                        if let message {
                            Text(message)
                                .font(.footnote)
                                .foregroundStyle(.secondary)
                                .multilineTextAlignment(.center)
                                .accessibilityIdentifier("paywall.message")
                        }
                        if let closeNote, !store.isPro {
                            Text(closeNote)
                                .font(.footnote)
                                .foregroundStyle(.secondary)
                                .multilineTextAlignment(.center)
                                .accessibilityIdentifier("paywall.closeNote")
                        }
                        benefits
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
        .task {
            if store.offer == nil { await store.loadProducts() }
        }
        .onChange(of: store.isPro) { _, isPro in
            guard isPro else { return }
            justPurchased = true
            Task {
                try? await Task.sleep(for: .seconds(1.4))
                dismiss()
            }
        }
    }

    // MARK: - 見出し

    private var header: some View {
        VStack(spacing: Theme.Spacing.md) {
            BrandMark(size: 72)
            Text(verbatim: "Tailor It Pro")
                .font(.largeTitle.weight(.bold))
                .brandText()
            Text(headline)
                .font(.headline)
                .multilineTextAlignment(.center)
                .fixedSize(horizontal: false, vertical: true)
                .accessibilityIdentifier("paywall.headline")
        }
    }

    private var headline: String {
        switch reason {
        case .settings:
            return L10n.tr("共有・オンラインの機能と、新しいアプリを、いくつでも")
        case .limitReached:
            return L10n.tr("今日の無料作成(\(gate.quota.dailyFreeLimit) 回)を使い切りました")
        case .paidFeature(let features):
            return L10n.tr("「\(Self.featureNames(features))」には Tailor It Pro が必要です")
        case .join(let name):
            return L10n.tr("「\(name)」に参加するには Tailor It Pro が必要です")
        }
    }

    /// カードと同じ機能名(backend の title)。生成の後の判定など、名前が無い時は「みんなで共有するデータ」。
    static func featureNames(_ features: [PaidFeature]) -> String {
        let names = features.map(\.title).filter { !$0.isEmpty }
        return names.isEmpty ? L10n.tr("みんなで共有するデータ") : names.joined(separator: "・")
    }

    private func featureReasons(_ features: [PaidFeature]) -> some View {
        let reasons = features.map(\.reason).filter { !$0.isEmpty }
        let lines = reasons.isEmpty ? [L10n.tr("みんなで書き込むデータはクラウドに保存して全員の端末に同期するため、サーバーの費用がかかります。")] : reasons
        return VStack(alignment: .leading, spacing: Theme.Spacing.sm) {
            ForEach(lines, id: \.self) { line in
                Text(line)
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .accessibilityIdentifier("paywall.reason")
    }

    // MARK: - できるようになること

    private var benefits: some View {
        VStack(alignment: .leading, spacing: Theme.Spacing.md) {
            Text("Pro でできること")
                .font(.subheadline.weight(.semibold))
                .foregroundStyle(.secondary)
            benefit("person.2.fill", "みんなで書き込める共有のデータ(掲示板・ランキング・割り勘など)")
            benefit("link", "シェアされたアプリに参加して、同じデータを使えます")
            benefit("infinity", "毎日、新しいアプリを無制限に作れます")
            benefit("waveform", "作ったアプリへの「ここを変えて」はこれまでどおり無料")
            benefit("arrow.uturn.backward.circle", "いつでも解約できます(App Store の設定から)")
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .metamonCard()
        .accessibilityIdentifier("paywall.benefits")
    }

    private func benefit(_ symbol: String, _ text: LocalizedStringKey) -> some View {
        HStack(alignment: .firstTextBaseline, spacing: Theme.Spacing.md) {
            Image(systemName: symbol)
                .font(.body.weight(.semibold))
                .foregroundStyle(Theme.brandGradient)
                .frame(width: 24)
            Text(text).font(.body)
                .fixedSize(horizontal: false, vertical: true)
        }
    }

    // MARK: - 価格・購入・復元

    private var purchaseArea: some View {
        VStack(spacing: Theme.Spacing.md) {
            planRow
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
            .disabled(busy || store.offer == nil)
            .accessibilityIdentifier("paywall.purchase")

            Button("購入を復元") { Task { await restore() } }
                .font(.subheadline.weight(.medium))
                .foregroundStyle(Theme.Colors.brandText)
                .disabled(busy)
                .accessibilityIdentifier("paywall.restore")

            if store.offer == nil, !store.isLoadingProducts {
                Text("価格を取得できませんでした。通信状態を確認してください。")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }
        }
    }

    /// 3.1.2: サブスクの名前・期間・価格をボタンの外にも出す
    private var planRow: some View {
        HStack(alignment: .firstTextBaseline) {
            VStack(alignment: .leading, spacing: Theme.Spacing.xs) {
                Text(verbatim: store.offer?.displayName ?? "Tailor It Pro")
                    .font(.headline)
                Text("1 か月ごとの自動更新")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            Spacer()
            if let offer = store.offer {
                Text(L10n.tr("\(offer.displayPrice) / 月"))
                    .font(.title3.weight(.bold))
                    .accessibilityIdentifier("paywall.price")
            } else if store.isLoadingProducts {
                ProgressView()
            }
        }
        .frame(maxWidth: .infinity)
        .metamonCard()
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

    /// 3.1.2: 自動更新の説明 + 利用規約・プライバシーポリシーへのリンク
    private var finePrint: some View {
        VStack(spacing: Theme.Spacing.sm) {
            Text("月額の自動更新サブスクリプションです。お支払いは購入の確定時に Apple ID に請求されます。期間終了の 24 時間前までに解約しない限り自動で更新され、更新分は終了前の 24 時間以内に請求されます。解約は App Store のアカウント設定からいつでもできます。")
                .font(.caption2)
                .foregroundStyle(.secondary)
                .multilineTextAlignment(.center)
            HStack(spacing: Theme.Spacing.lg) {
                LegalDocumentLink(document: .terms)
                LegalDocumentLink(document: .privacyPolicy)
            }
            .font(.caption)
            .labelStyle(.titleOnly)
            .tint(Theme.Colors.brandText)
        }
    }

    private var priceLabel: String {
        guard let offer = store.offer else { return L10n.tr("Pro にする") }
        return L10n.tr("Pro にする(\(offer.displayPrice) / 月)")
    }

    private func buy() async {
        guard store.offer != nil else { return }
        busy = true
        defer { busy = false }
        do {
            switch try await store.purchaseOffer() {
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

/// `ProGate.requestPro` の課金画面を present する。`placement` が一致する要求だけ出す
/// (root = RootView の外側・chat = 確認カードのある対話画面。音声シートの中でも重ねて出せる)。
struct PaywallPresenter: ViewModifier {
    let placement: PaywallRequest.Placement

    func body(content: Content) -> some View {
        let gate = ProGate.shared
        return content.sheet(item: Binding(
            get: { gate.paywall?.placement == placement ? gate.paywall : nil },
            set: { if $0 == nil { gate.dismissPaywall(placement: placement) } }
        ), onDismiss: { gate.dismissPaywall(placement: placement) }) { request in
            PaywallSheet(reason: request.reason, closeNote: request.closeNote)
        }
    }
}

/// 課金画面(上限・有料機能・参加)の present 元 + 起動時のストア開始。RootView の外側(MetamonApp)に 1 つだけ付ける。
struct PaywallHost: ViewModifier {
    func body(content: Content) -> some View {
        content
            .modifier(PaywallPresenter(placement: .root))
            .task { ProGate.shared.start() }
    }
}

#Preview {
    PaywallSheet(reason: .paidFeature([]), closeNote: "閉じると、共有の機能を外して自分だけの記録にします。")
}
#endif
