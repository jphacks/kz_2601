#if !APP_STORE_REVIEW // 審査用ビルドは App 内課金を持たない(#450)
import SwiftUI
import MetamonRenderer
import MetamonServices

/// 設定の「コイン」(#630): 残高・今月の枠・「枠を超えたらコインで続ける」(既定 OFF)と月の上限・履歴・お店。
/// サーバーがコイン課金を有効にしていなければ何も出さない。
struct CoinSettingsSection: View {
    /// 断りから開いた時の説明(枠・上限)
    var reason: CoinDenial?

    @State private var overflowOn = false
    @State private var cap = 0
    @State private var saving = false
    @State private var saveError = false
    @State private var synced = false
    @State private var saveTask: Task<Void, Never>?

    private var gate: CoinGate { CoinGate.shared }
    private var wallet: CoinWallet? { gate.store.wallet }

    var body: some View {
        if gate.isEnabled {
            if let reason {
                Section {
                    VStack(alignment: .leading, spacing: Theme.Spacing.xs) {
                        Label(CoinCopy.denialTitle(reason.kind), systemImage: "exclamationmark.circle.fill")
                            .font(.headline)
                            .foregroundStyle(Theme.Colors.warning)
                        Text(CoinCopy.denialBody(reason)).font(.subheadline)
                    }
                    .accessibilityElement(children: .combine)
                    .accessibilityIdentifier("coins.settings.reason")
                }
            }
            balanceSection
            overflowSection
        }
    }

    private var balanceSection: some View {
        Section {
            LabeledContent(CoinCopy.balanceTitle) { CoinBalanceChip(accessibilityID: "settings.coins.balance") }
            if let b = wallet?.balance {
                LabeledContent(CoinCopy.purchasedTitle) { Text(CoinCopy.coins(b.purchased)).monospacedDigit() }
                if b.grant > 0 {
                    LabeledContent(CoinCopy.grantTitle(until: b.grantPeriodEnd)) { Text(CoinCopy.coins(b.grant)).monospacedDigit() }
                }
            }
            if let wallet {
                ForEach(CoinCopy.orderedAllowanceKeys(wallet.allowance.keys), id: \.self) { key in
                    if let name = CoinCopy.allowanceName(key), let m = wallet.allowance[key] {
                        LabeledContent(L10n.tr("今月の\(name)")) {
                            Text(CoinCopy.meter(m))
                                .monospacedDigit()
                                .foregroundStyle(UsageNoticeRules.isNear(m) ? AnyShapeStyle(Theme.Colors.warning) : AnyShapeStyle(.secondary))
                        }
                        .accessibilityIdentifier("settings.coins.allowance.\(key)")
                    }
                }
            }
            NavigationLink {
                CoinHistoryView()
            } label: {
                Label("コインの履歴", systemImage: "list.bullet.rectangle")
            }
            .accessibilityIdentifier("settings.coins.history")
            Button { gate.openShop() } label: { Label("コインを買う", systemImage: "plus.circle") }
                .accessibilityIdentifier("settings.coins.shop")
            Button { gate.openPlans() } label: { Label("プランを比べる", systemImage: "square.split.2x1") }
                .accessibilityIdentifier("settings.coins.plans")
        } header: {
            Text("コイン")
        } footer: {
            Text("AI で作る・直す・アプリの中の AI は、使うたびにコインを使います。送る前に目安を出します。")
        }
    }

    private var overflowSection: some View {
        Section {
            Toggle("枠を超えたらコインで続ける", isOn: Binding(get: { overflowOn }, set: { on in
                overflowOn = on
                commit()
            }))
            .tint(Theme.Colors.brandA)
            .disabled(wallet == nil)
            .accessibilityIdentifier("settings.coins.overflow")
            if overflowOn {
                Stepper(value: Binding(get: { cap }, set: { v in
                    cap = max(0, v)
                    commit(after: .milliseconds(700)) // 続けて押した分はまとめて送る
                }), in: 0...Int.max, step: capStep) {
                    LabeledContent(L10n.tr("月の上限")) { Text(CoinCopy.coins(cap)).monospacedDigit() }
                }
                .accessibilityIdentifier("settings.coins.cap")
                if let spent = wallet?.overflow.spent {
                    LabeledContent(L10n.tr("今月コインで払った分")) { Text(CoinCopy.coins(spent)).monospacedDigit().foregroundStyle(.secondary) }
                }
            }
            if saving {
                HStack { ProgressView(); Text("保存しています…").foregroundStyle(.secondary) }
            }
        } footer: {
            Text(saveError
                 ? L10n.tr("保存できませんでした。時間をおいて試してください。")
                 : L10n.tr("自動の動き・通知などの今月の枠を使い切った時、ON なら決めた上限までコインで続けます。OFF なら止めて知らせます。"))
        }
        .onAppear { syncFromWallet() }
        .onChange(of: wallet) { _, _ in if !saving { syncFromWallet() } }
    }

    private var capStep: Int { max(1, gate.store.config?.overflowBlockCoins ?? 10) }

    private func syncFromWallet() {
        guard let o = wallet?.overflow else { return }
        overflowOn = o.enabled
        cap = o.monthlyCap ?? 0
        synced = true
    }

    /// 楽観的に変えて送る。失敗したらサーバーの値に戻す
    private func commit(after delay: Duration = .zero) {
        guard synced else { return }
        saveTask?.cancel()
        let on = overflowOn, value = cap
        saveTask = Task {
            if delay > .zero {
                try? await Task.sleep(for: delay)
                if Task.isCancelled { return }
            }
            saving = true
            defer { saving = false }
            do {
                try await gate.store.updateOverflow(enabled: on, monthlyCap: value)
                saveError = false
            } catch {
                saveError = true
                print("[Metamon] coins: settings failed: \(error)")
                syncFromWallet()
            }
        }
    }
}

/// 断り(枠・上限)から開く「コインの設定」。設定シートの同じセクションだけを出す。
struct CoinSettingsSheet: View {
    var reason: CoinDenial?
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        NavigationStack {
            List { CoinSettingsSection(reason: reason) }
                .navigationTitle(Text("コイン"))
                .navigationBarTitleDisplayMode(.inline)
                .toolbar { SheetCloseToolbarItem(identifier: "coins.settings.close") { dismiss() } }
                .task { await CoinGate.shared.store.refreshWallet() }
        }
        .presentationDetents([.medium, .large])
        .presentationDragIndicator(.visible)
    }
}

/// コインの履歴(`GET /v1/me/coins/ledger`)。
struct CoinHistoryView: View {
    @State private var entries: [CoinLedgerEntry] = []
    @State private var cursor: String?
    @State private var loading = true
    @State private var failed = false

    var body: some View {
        List {
            if loading && entries.isEmpty {
                ForEach(0..<5, id: \.self) { _ in
                    HStack { CoinSkeleton(width: 140); Spacer(); CoinSkeleton(width: 40) }
                }
            } else if entries.isEmpty {
                Text(failed ? L10n.tr("履歴を読み込めませんでした。") : L10n.tr("まだ履歴はありません。"))
                    .foregroundStyle(.secondary)
            } else {
                ForEach(entries) { e in row(e) }
                if cursor != nil {
                    ProgressView()
                        .frame(maxWidth: .infinity)
                        .task { await load() }
                }
            }
        }
        .navigationTitle(Text("コインの履歴"))
        .task { if entries.isEmpty { await load() } }
        .refreshable {
            entries = []
            cursor = nil
            await load()
        }
    }

    private func row(_ e: CoinLedgerEntry) -> some View {
        HStack(alignment: .firstTextBaseline) {
            VStack(alignment: .leading, spacing: 2) {
                Text(verbatim: [CoinCopy.ledgerKind(e.kind), CoinCopy.operationName(e.operation), e.appName]
                    .compactMap { $0 }.filter { !$0.isEmpty }.joined(separator: " · "))
                if let at = e.at {
                    Text(at.formatted(date: .abbreviated, time: .shortened)).font(.caption).foregroundStyle(.secondary)
                }
            }
            Spacer()
            Text(verbatim: CoinCopy.signedDelta(e.delta))
                .font(.body.weight(.semibold))
                .monospacedDigit()
                .foregroundStyle(e.delta > 0 ? AnyShapeStyle(Theme.Colors.brandText) : AnyShapeStyle(.primary))
        }
        .accessibilityElement(children: .combine)
    }

    private func load() async {
        guard let service = CoinGate.shared.store.service else {
            loading = false
            return
        }
        loading = true
        defer { loading = false }
        do {
            let page = try await service.ledger(cursor: cursor)
            entries += page.entries
            cursor = page.nextCursor
            failed = false
        } catch {
            failed = true
            cursor = nil
            print("[Metamon] coins: ledger failed: \(error)")
        }
    }
}
#endif
