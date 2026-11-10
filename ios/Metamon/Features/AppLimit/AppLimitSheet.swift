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

/// 置き換えシートの文言(ja・en は String Catalog)。
enum AppLimitCopy {
    static var untitled: String { L10n.tr("名前のないアプリ") }
    static var deleteLabel: String { L10n.tr("削除") }
    static var leaveLabel: String { L10n.tr("抜ける") }
    static var slotFailed: String { L10n.tr("枠を買えませんでした") }

    static func subtitle(plan: AppReplacePlan) -> String {
        let need = plan.removalsNeeded
        return need > 1
            ? L10n.tr("持てるアプリは \(plan.limit) 個までです(今 \(plan.held) 個)。あと \(need) 個を削除するか抜けると、続けられます。")
            : L10n.tr("持てるアプリは \(plan.limit) 個までです(今 \(plan.held) 個)。1 つ削除するか抜けると、続けられます。")
    }

    static func detail(_ row: AppReplacePlan.Row) -> String {
        let role = row.role == .own ? L10n.tr("自分で作成") : L10n.tr("参加中")
        let opened: String
        if row.isCurrent {
            opened = L10n.tr("使用中")
        } else if let at = row.lastOpened {
            opened = L10n.tr("最後に開いた: \(at.formatted(.relative(presentation: .named)))")
        } else {
            opened = L10n.tr("まだ開いていません")
        }
        return "\(role) ・ \(opened)"
    }

    static func confirmTitle(_ row: AppReplacePlan.Row) -> String {
        let name = row.name.isEmpty ? untitled : row.name
        return row.role == .own ? L10n.tr("「\(name)」を削除しますか?") : L10n.tr("「\(name)」から抜けますか?")
    }

    static func confirmMessage(_ row: AppReplacePlan.Row) -> String {
        row.role == .own
            ? L10n.tr("元に戻せません。このアプリの記録(データ)もすべて消えます。シェアリンクと「探す」への掲載も取り下げ、共有していれば参加している人も使えなくなります。")
            : L10n.tr("この端末の一覧から消え、参加している人の記録は見られなくなります。もう一度参加するには、シェアリンクを開き直してください。")
    }

    static func confirmButton(_ row: AppReplacePlan.Row) -> String {
        row.role == .own ? L10n.tr("削除して続ける") : L10n.tr("抜けて続ける")
    }

    static func buySlot(coins: Int) -> String { L10n.tr("枠を 1 つ買う(\(coins) コイン)") }

    static func slotBought(total: Int) -> String { L10n.tr("アプリの枠を 1 つ増やしました(買った枠 \(total) 個)") }

    /// シートを出せない所(画面の外)で断られた時の 1 文
    static func toast(_ limit: AppLimit) -> String {
        L10n.tr("持てるアプリの数(\(limit.limit) 個)に達しました。使わないアプリを削除してから、もう一度お試しください")
    }

    // 状態カード(生成・チャットで断られた時)
    static var cardTitle: String { L10n.tr("持てるアプリの数に達しました") }
    static func cardBody(_ limit: AppLimit) -> String {
        L10n.tr("持てるアプリは \(limit.limit) 個までです。どれかと置き換えると、続けて作れます。")
    }
    static var replaceButton: String { L10n.tr("置き換える") }
}
