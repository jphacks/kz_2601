import SwiftUI
import MetamonServices

/// Mac で置き換えに使えるアプリ(この Mac で作ったアプリ)と、その消し方(`MacRootView` の削除と同じ)。
/// 会話のシートに環境で渡す(`MacErrorCard` の「置き換える」が読む)。
struct MacAppReplaceContext {
    var apps: [AppSummary]
    /// 中央で開いているアプリ(一覧の最後に回す)
    var current: String?
    var delete: @MainActor (String) -> Void
}

extension EnvironmentValues {
    @Entry var macAppReplace: MacAppReplaceContext?
}

/// 持てるアプリの数の上限(#630・402 `app_limit_reached`)の置き換えシート(iOS の `AppLimitSheet` の Mac 版)。
/// どれかを削除すると(確認を挟む)、閉じて元の操作をやり直す。並びと文言は iOS と同じ `AppReplacePlan` / `AppLimitCopy`。
/// Mac はサーバーにつながっていない(#930)ので、消せるのはこの Mac で作ったアプリだけ。共有から抜ける・枠を買う(コイン)は出さない。
struct MacAppLimitSheet: View {
    let limit: AppLimit
    let context: MacAppReplaceContext
    /// 1 つ空いた後(元の操作をやり直す)
    var onFreed: () -> Void
    @Environment(\.dismiss) private var dismiss
    @State private var plan: AppReplacePlan?
    @State private var confirming: AppReplacePlan.Row?

    var body: some View {
        VStack(alignment: .leading, spacing: MacTheme.Spacing.group) {
            header
            if let plan, !plan.rows.isEmpty {
                ScrollView {
                    VStack(spacing: 8) {
                        ForEach(plan.rows) { row($0) }
                    }
                }
            } else {
                Text("この Mac で作ったアプリがありません。iPhone の Tailor It で使わないアプリを削除してから、もう一度お試しください。")
                    .font(.callout)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }
            HStack {
                Spacer()
                Button("キャンセル") { dismiss() }
                    .keyboardShortcut(.cancelAction)
                    .accessibilityIdentifier("appLimit.cancel")
            }
        }
        .padding(20)
        .frame(width: 460)
        .frame(minHeight: 280, maxHeight: 520)
        .onAppear { if plan == nil { plan = Self.makePlan(limit, context: context) } }
        .alert(confirming.map(AppLimitCopy.confirmTitle) ?? "", isPresented: Binding(
            get: { confirming != nil }, set: { if !$0 { confirming = nil } }
        ), presenting: confirming) { row in
            Button(AppLimitCopy.confirmButton(row), role: .destructive) { replace(row) }
                .accessibilityIdentifier("appLimit.confirm")
            Button("キャンセル", role: .cancel) {}
        } message: { _ in
            Text("元に戻せません。このアプリの記録(データ)もこの Mac から消えます。")
        }
    }

    private var header: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("上限に達しています。どのアプリと置き換えますか?")
                .font(.title3.weight(.bold))
                .fixedSize(horizontal: false, vertical: true)
                .accessibilityAddTraits(.isHeader)
                .accessibilityIdentifier("appLimit.title")
            Text(AppLimitCopy.subtitle(plan: plan ?? Self.makePlan(limit, context: context)))
                .font(.callout)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
        }
    }

    private func row(_ row: AppReplacePlan.Row) -> some View {
        HStack(alignment: .center, spacing: MacTheme.Spacing.group) {
            VStack(alignment: .leading, spacing: 2) {
                Text(row.name.isEmpty ? AppLimitCopy.untitled : row.name)
                    .font(.headline)
                    .lineLimit(1)
                Text(AppLimitCopy.detail(row))
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            Spacer(minLength: 0)
            Button(AppLimitCopy.deleteLabel, role: .destructive) { confirming = row }
                .help(AppLimitCopy.confirmTitle(row))
                .accessibilityIdentifier("appLimit.row.\(row.id)")
        }
        .padding(12)
        .background(.quaternary.opacity(0.5), in: RoundedRectangle(cornerRadius: 10, style: .continuous))
        .accessibilityElement(children: .combine)
    }

    /// この Mac で作ったアプリだけで並べる(サーバーの一覧はこの Mac から消せないので使わない)
    static func makePlan(_ limit: AppLimit, context: MacAppReplaceContext) -> AppReplacePlan {
        var local = limit
        local.apps = nil
        return AppReplacePlan.make(local, local: context.apps, lastOpened: { _ in nil },
                                   current: context.current, coinsEnabled: false)
    }

    private func replace(_ row: AppReplacePlan.Row) {
        context.delete(row.id)
        if let next = plan?.removing(row.id) {
            plan = next // まだ足りない(上限を下げた後など)
        } else {
            dismiss()
            onFreed()
        }
    }
}
