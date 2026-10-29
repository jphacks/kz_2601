import SwiftUI
import MetamonServices

/// 設定シートの「Tailor It Pro」セクション(#77): プランの状態・今日の回数・購入 / 復元。
struct ProSettingsSection: View {
    @State private var showPaywall = false
    @State private var restoring = false
    @State private var note: String?

    var body: some View {
        @Bindable var gate = ProGate.shared
        return Section {
            LabeledContent("プラン") {
                Text(gate.isPro ? "Pro" : L10n.tr("無料"))
                    .foregroundStyle(gate.isPro ? Theme.Colors.brandText : .secondary)
                    .fontWeight(gate.isPro ? .semibold : .regular)
            }
            .accessibilityIdentifier("settings.pro.plan")
            if gate.isPro {
                if let expires = gate.store.expiresAt {
                    LabeledContent("次回更新", value: expires.formatted(date: .abbreviated, time: .omitted))
                }
            } else {
                LabeledContent("今日の新規作成") {
                    Text(gate.isEnforced ? L10n.tr("\(min(gate.usedToday, gate.quota.dailyFreeLimit)) / \(gate.quota.dailyFreeLimit) 回") : L10n.tr("\(gate.usedToday) 回(制限なし)"))
                }
                Button {
                    showPaywall = true
                } label: {
                    Label("Pro にする", systemImage: "sparkles")
                }
                .accessibilityIdentifier("settings.pro.upgrade")
            }
            Button {
                Task { await restore() }
            } label: {
                Label("購入を復元", systemImage: "arrow.clockwise")
            }
            .disabled(restoring)
            #if DEBUG
            Toggle("課金の制限を無効にする(デモ用・回数と有料機能)", isOn: $gate.limitDisabled)
                .tint(Theme.Colors.brandA)
                .accessibilityIdentifier("settings.pro.limitDisabled")
            #endif
        } header: {
            Text(verbatim: "Tailor It Pro")
        } footer: {
            Text(note ?? L10n.tr("無料プランでは新しいアプリを 1 日 \(gate.quota.dailyFreeLimit) 回まで作れます。作ったアプリの改変は回数に数えません。"))
        }
        .sheet(isPresented: $showPaywall) { PaywallSheet(reason: .settings) }
    }

    private func restore() async {
        restoring = true
        defer { restoring = false }
        do {
            try await ProGate.shared.store.restore()
            note = ProGate.shared.isPro ? L10n.tr("購入を復元しました。") : L10n.tr("復元できる購入は見つかりませんでした。")
        } catch {
            note = L10n.tr("復元できませんでした。時間をおいて試してください。")
        }
    }
}
