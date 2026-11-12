#if METAMON_STG
import SwiftUI
import MetamonServices

/// 開発用メニュー: 試験用のアカウントの枠に切り替える(#2084)
final class DebugAccountSection: DebugMenuSection {
    override class var order: Int { 50 }
    override class var title: String { "アカウントの枠" }
    override class var footer: String? {
        "枠ごとに別のサインイン(匿名)を端末のキーチェーンに持ちます。枠 0 がいつものアカウントです。切り替えると端末のデータ(送っていない記録も)を消してアプリを終了します。開き直すとその枠でサインインしています。「全部元に戻す」では枠は変わりません。"
    }
    @MainActor override class func makeBody() -> AnyView { AnyView(DebugAccountRows()) }
}

private struct DebugAccountRows: View {
    @Environment(AppRuntime.self) private var rt
    @Environment(AuthSession.self) private var session
    @State private var copied: Int?
    @State private var confirm: Int?
    @State private var switching = false

    private let current = DebugAuthSlot.current

    var body: some View {
        ForEach(Array(DebugAuthSlot.slots), id: \.self) { slot in
            row(slot)
        }
        .confirmationDialog(
            Text(verbatim: "スロット \(confirm ?? 0) に切り替えますか？"),
            isPresented: Binding(get: { confirm != nil }, set: { if !$0 { confirm = nil } }),
            titleVisibility: .visible
        ) {
            Button(role: .destructive) {
                if let slot = confirm { switchTo(slot) }
            } label: { Text(verbatim: "データを消して終了する") }
            Button(role: .cancel) {} label: { Text(verbatim: "キャンセル") }
        } message: {
            Text(verbatim: "この端末のアプリと記録(送っていないものも)を消してアプリを終了します。今の枠のサインインは残ります。")
        }
    }

    private func user(_ slot: Int) -> AuthUser? {
        slot == current ? session.user : FirebaseBootstrap.currentUser(appName: DebugAuthSlot.appName(for: slot))
    }

    @ViewBuilder
    private func row(_ slot: Int) -> some View {
        let user = user(slot)
        VStack(alignment: .leading, spacing: 6) {
            HStack {
                Text(verbatim: slot == 0 ? "スロット 0(いつもの)" : "スロット \(slot)(試験用)")
                if slot == current {
                    Text(verbatim: "使用中")
                        .font(.caption.bold())
                        .foregroundStyle(Theme.Colors.brandA)
                }
            }
            HStack {
                Text(verbatim: user?.uid ?? "未サインイン")
                    .font(.footnote.monospaced())
                    .lineLimit(1)
                    .truncationMode(.middle)
                    .textSelection(.enabled)
                if let uid = user?.uid {
                    Button {
                        UIPasteboard.general.string = uid
                        copied = slot
                    } label: {
                        Image(systemName: copied == slot ? "checkmark" : "doc.on.doc")
                    }
                    .buttonStyle(.borderless)
                    .accessibilityLabel(Text(verbatim: "uid をコピー"))
                    .accessibilityIdentifier("debugMenu.account.copy.\(slot)")
                }
            }
            Text(verbatim: user.map { $0.isAnonymous ? "匿名" : "Apple" } ?? "-")
                .font(.caption)
                .foregroundStyle(.secondary)
            if slot != current {
                Button { confirm = slot } label: {
                    DebugKnobLabel(title: "このスロットに切り替える (再起動)", effect: .relaunch)
                }
                .buttonStyle(.borderless)
                .disabled(switching)
                .accessibilityIdentifier("debugMenu.account.switch.\(slot)")
            }
        }
        .accessibilityIdentifier("debugMenu.account.slot.\(slot)")
    }

    private func switchTo(_ slot: Int) {
        switching = true
        Task {
            await rt.settlePendingRecordDeletes() // 取り消し待ちの削除は今の枠で送る(ログアウトと同じ)
            await DebugAuthSlot.switchTo(slot, defaults: DebugKnobs.store.defaults,
                                         wipe: { await rt.wipeBeforeAccountChange() },
                                         relaunch: { exit(0) })
        }
    }
}
#endif
