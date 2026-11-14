#if METAMON_STG
import SwiftUI
import MetamonServices

/// 開発用メニュー: 試験用のアカウントの枠に切り替える(#2084)
final class DebugAccountSection: DebugMenuSection {
    override class var order: Int { 50 }
    override class var title: String { "アカウントの枠" }
    override class var footer: String? {
        "枠ごとに別のサインイン(匿名)を端末のキーチェーンに持ちます。枠 0 がいつものアカウントです。切り替えると端末のデータ(送っていない記録も)を消してアプリを終了します。開き直すとその枠でサインインしています。試験用の枠では、いつもの Apple ID と結びつかないように Apple への引き継ぎとアカウントの削除を出しません。「全部元に戻す」では枠は変わりません。"
    }
    @MainActor override class func makeBody() -> AnyView { AnyView(DebugAccountRows()) }
}

private struct DebugAccountRows: View {
    @Environment(AppRuntime.self) private var rt
    @Environment(AuthSession.self) private var session
    @State private var copied: Int?
    @State private var confirm: Int?
    @State private var confirmRenew: Int?
    @State private var renewed: [Int: AuthUser] = [:]
    @State private var renewError: String?
    @State private var switching = false

    private let current = DebugAuthSlot.current

    var body: some View {
        ForEach(Array(DebugAuthSlot.slots), id: \.self) { slot in
            row(slot)
        }
        .confirmationDialog(
            Text(verbatim: "枠 \(confirm ?? 0) に切り替えますか？"),
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
        .confirmationDialog(
            Text(verbatim: "枠 \(confirmRenew ?? 0) を新しくしますか？"),
            isPresented: Binding(get: { confirmRenew != nil }, set: { if !$0 { confirmRenew = nil } }),
            titleVisibility: .visible
        ) {
            Button(role: .destructive) {
                if let slot = confirmRenew { renew(slot) }
            } label: { Text(verbatim: "新しい匿名にする") }
            Button(role: .cancel) {} label: { Text(verbatim: "キャンセル") }
        } message: {
            Text(verbatim: "この枠をサインアウトして、新しい匿名のアカウントで入り直します。前のアカウントには戻れません。使用中の枠なら端末のデータを消してアプリを終了します。")
        }
        if let renewError {
            Text(verbatim: renewError).font(.footnote).foregroundStyle(.red)
        }
    }

    private func user(_ slot: Int) -> AuthUser? {
        if let user = renewed[slot] { return user }
        return slot == current ? session.user : FirebaseBootstrap.currentUser(appName: DebugAuthSlot.appName(for: slot))
    }

    @ViewBuilder
    private func row(_ slot: Int) -> some View {
        let user = user(slot)
        VStack(alignment: .leading, spacing: 6) {
            HStack {
                Text(verbatim: slot == 0 ? "枠 0(いつもの)" : "枠 \(slot)(試験用)")
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
            Text(verbatim: Self.detail(user: user, lastUsed: DebugAuthSlot.lastUsed(slot)))
                .font(.caption)
                .foregroundStyle(.secondary)
            if slot != current {
                Button { confirm = slot } label: {
                    DebugKnobLabel(title: "この枠にする(開き直す)", effect: .relaunch)
                }
                .buttonStyle(.borderless)
                .disabled(switching)
                .accessibilityIdentifier("debugMenu.account.switch.\(slot)")
            }
            if slot != 0 { // 枠 0(いつものアカウント)は新しくしない
                Button { confirmRenew = slot } label: {
                    DebugKnobLabel(title: "この枠を新しくする(新しい匿名)", effect: slot == current ? .relaunch : .immediate)
                }
                .buttonStyle(.borderless)
                .disabled(switching)
                .accessibilityIdentifier("debugMenu.account.renew.\(slot)")
            }
        }
        .accessibilityIdentifier("debugMenu.account.slot.\(slot)")
    }

    nonisolated static func detail(user: AuthUser?, lastUsed: Date?) -> String {
        let kind = user.map { $0.isAnonymous ? "匿名" : "Apple" } ?? "未サインイン"
        guard let lastUsed else { return "\(kind)・まだ使っていません" }
        return "\(kind)・最後に使った日時 \(lastUsed.formatted(.dateTime.locale(Locale(identifier: "ja_JP")).year().month().day().hour().minute()))"
    }

    private func renew(_ slot: Int) {
        guard let name = DebugAuthSlot.appName(for: slot) else { return }
        renewError = nil
        switching = true
        Task {
            if slot == current { // 使用中: 前のアカウントの端末のデータを消してから入り直し、開き直す
                await rt.settlePendingRecordDeletes()
                await rt.wipeBeforeAccountChange()
            }
            do {
                let user = try await FirebaseBootstrap.renewAnonymous(appName: name)
                if slot == current { exit(0) } // 開き直すと新しい匿名で入っている
                renewed[slot] = user
            } catch {
                renewError = "新しくできませんでした: \(error.localizedDescription)"
                if slot == current { exit(0) } // 端末のデータはもう消えている。開き直すとこの枠のサインインからやり直せる
            }
            switching = false
        }
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
