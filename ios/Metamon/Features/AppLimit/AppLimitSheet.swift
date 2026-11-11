import SwiftUI
import MetamonServices

/// 持てるアプリの数の上限(#630)の置き換えシート。どれかを削除する / 抜ける、または枠を 1 つ買うと、元の操作をやり直す。
/// 削除・抜けるは確認を出してから(自分のアプリはデータも消え、共有していれば参加している人も使えなくなる)。
struct AppLimitSheet: View {
    let request: AppLimitGate.Request
    @Environment(AppRuntime.self) private var rt
    @Environment(\.dismiss) private var dismiss
    @State private var plan: AppReplacePlan?
    @State private var confirming: AppReplacePlan.Row?
    @State private var busy = false
    /// 枠の購入の冪等キー(押した 1 回ごと。失敗した送り直しは同じ値)
    @State private var slotRequestID = UUID().uuidString

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: Theme.Spacing.lg) {
                    header
                    if let plan {
                        rows(plan)
                        if let slot = plan.slot { slotButton(slot) }
                    }
                }
                .padding(Theme.Spacing.lg)
            }
            .background(Theme.Colors.canvas.ignoresSafeArea())
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("キャンセル") { dismiss() }
                        .disabled(busy)
                        .accessibilityIdentifier("appLimit.cancel")
                }
            }
            .overlay { if busy { ProgressView().controlSize(.large) } }
        }
        .presentationDetents([.medium, .large])
        .presentationDragIndicator(.visible)
        .interactiveDismissDisabled(busy)
        #if !APP_STORE_REVIEW
        .coinSheets() // 枠を買うのにコインが足りない時のお店をこの上に出す
        #endif
        .onAppear { if plan == nil { plan = makePlan() } }
        .alert(confirming.map(AppLimitCopy.confirmTitle) ?? "", isPresented: Binding(
            get: { confirming != nil }, set: { if !$0 { confirming = nil } }
        ), presenting: confirming) { row in
            Button(AppLimitCopy.confirmButton(row), role: .destructive) { Task { await replace(row) } }
                .accessibilityIdentifier("appLimit.confirm")
            Button("キャンセル", role: .cancel) {}
        } message: { row in
            Text(AppLimitCopy.confirmMessage(row))
        }
    }

    private var header: some View {
        VStack(alignment: .leading, spacing: Theme.Spacing.sm) {
            Text("上限に達しています。どのアプリと置き換えますか?")
                .font(.system(size: 20, weight: .bold, design: .rounded))
                .fixedSize(horizontal: false, vertical: true)
                .accessibilityIdentifier("appLimit.title")
            Text(AppLimitCopy.subtitle(plan: plan ?? makePlan()))
                .font(.footnote)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
        }
    }

    private func rows(_ plan: AppReplacePlan) -> some View {
        VStack(spacing: Theme.Spacing.sm) {
            ForEach(plan.rows) { row in
                HStack(alignment: .center, spacing: Theme.Spacing.md) {
                    VStack(alignment: .leading, spacing: 2) {
                        Text(row.name.isEmpty ? AppLimitCopy.untitled : row.name)
                            .font(.subheadline.weight(.semibold))
                            .lineLimit(1)
                        Text(AppLimitCopy.detail(row))
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                    Spacer(minLength: 0)
                    Button(role: .destructive) { confirming = row } label: {
                        Text(row.role == .own ? AppLimitCopy.deleteLabel : AppLimitCopy.leaveLabel)
                            .font(.footnote.weight(.semibold))
                    }
                    .buttonStyle(.bordered)
                    .disabled(busy || !rt.canDelete)
                    .accessibilityIdentifier("appLimit.row.\(row.id)")
                }
                .metamonCard(padding: Theme.Spacing.md)
            }
        }
    }

    private func slotButton(_ slot: AppReplacePlan.SlotOffer) -> some View {
        Button { Task { await buySlot() } } label: {
            Label(AppLimitCopy.buySlot(coins: slot.coins), systemImage: "plus.square.on.square")
                .font(.subheadline.weight(.semibold))
                .frame(maxWidth: .infinity)
        }
        .buttonStyle(.borderedProminent)
        .disabled(busy)
        .accessibilityIdentifier("appLimit.buySlot")
    }

    private func makePlan() -> AppReplacePlan {
        let opened = LastOpenedStore()
        #if APP_STORE_REVIEW
        let coins = false
        #else
        let coins = CoinGate.shared.isEnabled
        #endif
        return AppReplacePlan.make(request.limit, local: rt.apps, lastOpened: { opened.lastOpened(appId: $0) },
                                   current: rt.document.appId, coinsEnabled: coins)
    }

    private func replace(_ row: AppReplacePlan.Row) async {
        busy = true
        defer { busy = false }
        guard await rt.replaceHeldApp(row) else { return }
        guard let current = plan else { return }
        if let next = current.removing(row.id) {
            plan = next // まだ足りない(上限を下げた後など)
        } else {
            await finish()
        }
    }

    private func buySlot() async {
        #if !APP_STORE_REVIEW
        busy = true
        defer { busy = false }
        guard await rt.buyAppSlot(requestId: slotRequestID) else { return }
        slotRequestID = UUID().uuidString
        if let next = plan?.addingSlot() {
            plan = next
        } else {
            await finish()
        }
        #endif
    }

    /// 空いたので閉じて、元の操作をやり直す
    private func finish() async {
        let retry = request.retry
        dismiss()
        AppLimitGate.shared.dismiss()
        await retry()
    }
}
