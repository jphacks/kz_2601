// 審査用ビルド(APP_STORE_REVIEW・#450 G1)は App 内課金を持たない(iOS と同じ)。
#if !APP_STORE_REVIEW
import SwiftUI
import MetamonServices

/// 「Tailor It Pro」の課金画面(Mac)。iOS の `PaywallSheet` と同じ並び: 見出し → 価格と購入 → できるようになること →
/// 自動更新の説明・規約とプライバシー(3.1.2)。商品と購入は iOS と同じ(`MacPro`・`ProEntitlementStore`)。
/// コイン(#630)と無料作成の回数は Mac にまだ無いので出さない。
struct MacPaywallView: View {
    var reason: MacPaywallReason = .settings
    var closeNote: String?
    @Environment(\.dismiss) private var dismiss
    @State private var busy = false
    @State private var message: String?
    @State private var justPurchased = false

    private var pro: MacPro { MacPro.shared }
    private var store: ProEntitlementStore { pro.store }

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(spacing: Theme.Spacing.lg) {
                    header
                    if case .paidFeature(let features) = reason { featureReasons(features) }
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
                .padding(.horizontal, 32)
                .padding(.vertical, 28)
            }
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("閉じる") { dismiss() }
                        .keyboardShortcut(.cancelAction)
                        .accessibilityIdentifier("paywall.close")
                }
            }
        }
        .frame(width: 520, height: 640)
        .task {
            pro.start()
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
            CraneMascot(pose: .happy, size: 72, sway: 4)
                .accessibilityHidden(true)
            Text(verbatim: "Tailor It Pro")
                .font(.largeTitle.weight(.bold))
                .brandText()
            Text(Self.headline(reason))
                .font(.headline)
                .multilineTextAlignment(.center)
                .fixedSize(horizontal: false, vertical: true)
                .accessibilityIdentifier("paywall.headline")
        }
    }

    static func headline(_ reason: MacPaywallReason) -> String {
        switch reason {
        case .settings:
            return L10n.tr("共有・オンラインの機能と、新しいアプリを、いくつでも")
        case .planLimit:
            return L10n.tr("Pro にすると、無料プランの上限が増えます")
        case .paidFeature(let features):
            return L10n.tr("「\(featureNames(features))」には Tailor It Pro が必要です")
        }
    }

    /// カードと同じ機能名。名前が無い時は「みんなで共有するデータ」(iOS と同じ)
    static func featureNames(_ features: [PaidFeature]) -> String {
        let names = features.map(\.title).filter { !$0.isEmpty }
        return names.isEmpty ? L10n.tr("みんなで共有するデータ") : names.joined(separator: L10n.tr("・"))
    }

    private func featureReasons(_ features: [PaidFeature]) -> some View {
        let reasons = features.map(\.reason).filter { !$0.isEmpty }
        return VStack(alignment: .leading, spacing: Theme.Spacing.sm) {
            ForEach(reasons, id: \.self) { line in
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
            benefit("iphone.and.arrow.forward", "iPhone と Mac のどちらで購入しても、同じ Apple ID なら両方で使えます")
            benefit("arrow.uturn.backward.circle", "いつでも解約できます(App Store のアカウントから)")
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
                .accessibilityHidden(true)
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
                    if busy { ProgressView().controlSize(.small) }
                    Text(priceLabel).font(.headline)
                }
                .frame(maxWidth: .infinity)
            }
            .buttonStyle(.borderedProminent)
            .controlSize(.large)
            .keyboardShortcut(.defaultAction)
            .disabled(busy || store.offer == nil)
            .accessibilityIdentifier("paywall.purchase")

            HStack(spacing: Theme.Spacing.lg) {
                Button("購入を復元") { Task { await restore() } }
                    .disabled(busy)
                    .accessibilityIdentifier("paywall.restore")
                Button("サブスクリプションを管理") { MacPro.openManageSubscriptions() }
                    .help("App Store のアカウントのページを開きます")
                    .accessibilityIdentifier("paywall.manage")
            }
            .buttonStyle(.link)
            .font(.subheadline)

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
                ProgressView().controlSize(.small)
            }
        }
        .frame(maxWidth: .infinity)
        .metamonCard()
    }

    private var proBadge: some View {
        VStack(spacing: Theme.Spacing.md) {
            Label(justPurchased ? "Pro になりました" : "Pro をご利用中です", systemImage: "checkmark.seal.fill")
                .font(.headline)
                .foregroundStyle(Theme.Colors.brandText)
                .accessibilityIdentifier("paywall.pro")
            Button("サブスクリプションを管理") { MacPro.openManageSubscriptions() }
                .buttonStyle(.link)
                .help("App Store のアカウントのページを開きます")
                .accessibilityIdentifier("paywall.manage")
        }
        .padding(.vertical, Theme.Spacing.md)
        .frame(maxWidth: .infinity)
        .metamonCard()
    }

    /// 3.1.2: 自動更新の説明 + 利用規約・プライバシーポリシーへのリンク
    private var finePrint: some View {
        VStack(spacing: Theme.Spacing.sm) {
            Text("月額の自動更新サブスクリプションです。お支払いは購入の確定時に Apple ID に請求されます。期間終了の 24 時間前までに解約しない限り自動で更新され、更新分は終了前の 24 時間以内に請求されます。解約は App Store のアカウント設定からいつでもできます。")
                .font(.caption2)
                .foregroundStyle(.secondary)
                .multilineTextAlignment(.center)
                .fixedSize(horizontal: false, vertical: true)
            HStack(spacing: Theme.Spacing.lg) {
                LegalDocumentLink(document: .terms)
                LegalDocumentLink(document: .privacyPolicy)
            }
            .font(.caption)
            .labelStyle(.titleOnly)
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

/// 中央のアプリの部品(オンラインの AI)が求めた課金画面を出す(`MacPro.requestPro`)。メインのウインドウに 1 つだけ付ける。
struct MacPaywallPresenter: ViewModifier {
    func body(content: Content) -> some View {
        let pro = MacPro.shared
        content
            .sheet(item: Binding(get: { pro.paywall }, set: { if $0 == nil { pro.dismissPaywall() } })) { request in
                MacPaywallView(reason: request.reason, closeNote: request.closeNote)
            }
            .task { pro.start() }
    }
}

/// 設定・失敗のカードなど、その場で課金画面を出すボタン。Pro の時は出さない。
struct MacUpgradeProButton: View {
    var reason: MacPaywallReason = .settings
    var closeNote: String?
    var prominent = false
    @State private var shown = false

    var body: some View {
        if !MacPro.shared.isPro {
            Group {
                if prominent {
                    Button(ErrorCopy.upgradePro) { shown = true }.buttonStyle(.borderedProminent)
                } else {
                    Button(ErrorCopy.upgradePro) { shown = true }
                }
            }
            .accessibilityIdentifier("paywall.open")
            .sheet(isPresented: $shown) { MacPaywallView(reason: reason, closeNote: closeNote) }
        }
    }
}
#endif
