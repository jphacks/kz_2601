import SwiftUI
import MetamonServices

/// 一覧の記録の「削除しました・元に戻す」(M11)。
/// 削除は `RecordSyncEngine.deleteUndoably` で数秒だけ送らずに待ち、その間に戻せばサーバーには何も送らない
/// (記録の id・作成日時・作者が変わらない)。時間が過ぎたら普段の削除と同じく送信の列に積む(オフラインなら復帰後に送る)。
struct RecordUndo: Identifiable, Equatable {
    let ticket: DeletionTicket
    var id: UUID { ticket.id }

    /// 戻せる時間。VoiceOver では読み上げてから操作するので長めにする
    static var seconds: Double { UIAccessibility.isVoiceOverRunning ? 10 : 5 }
}

extension AppRuntime {
    /// 一覧から消し、トーストに「元に戻す」を出す。前のトーストの削除はそのまま時間が来たら送る。
    func deleteUndoably(entity: String, recordId: String) async {
        let appId = document.appId
        let ticket = await recordSync.deleteUndoably(appId: appId, entity: entity, recordId: recordId,
                                                     grace: .seconds(RecordUndo.seconds)).ticket
        await reloadLocal(entity, appId: appId) // 取り消し待ちは cached から除かれる
        withAnimation { recordUndo = RecordUndo(ticket: ticket) }
        AccessibilityNotification.Announcement(L10n.tr("削除しました")).post()
    }

    /// 「元に戻す」。もう送った(時間切れ・背面に回った)なら何もしない。
    func undoRecordDelete() async {
        guard let undo = recordUndo else { return }
        withAnimation { recordUndo = nil }
        guard await recordSync.undoDelete(undo.ticket) != nil else { return }
        await reloadLocal(undo.ticket.entity, appId: undo.ticket.appId) // 別のアプリに切り替えた後なら読まない
    }

    /// ログアウト・アカウント削除の前: 「元に戻す」を閉じ、取り消し待ちの削除を待たずに送信の列へ積む
    /// (オンラインならいまのアカウントで送る。オフラインなら列に残り、ログアウトの確認の未送信件数に入る → 送らないなら端末の削除と一緒に消える)。
    func settlePendingRecordDeletes() async {
        withAnimation { recordUndo = nil }
        await recordSync.commitPendingDeletions()
    }
}

/// 下部の「削除しました・元に戻す」。トーストと同じカプセルで、`RecordUndo.seconds` で消える(消えた時に送られる)。
struct RecordUndoToast: View {
    @Environment(AppRuntime.self) private var rt
    let undo: RecordUndo

    var body: some View {
        let dismiss = { withAnimation { if rt.recordUndo == undo { rt.recordUndo = nil } } }
        HStack(spacing: Theme.Spacing.md) {
            Text("削除しました")
                .font(.footnote.weight(.medium))
            Button { Task { await rt.undoRecordDelete() } } label: {
                Text(UndoCopy.undo).font(.footnote.weight(.semibold))
            }
            .accessibilityIdentifier("record.undoDelete")
        }
        .padding(.horizontal, Theme.Spacing.md).padding(.vertical, Theme.Spacing.sm + 2)
        .background(.regularMaterial, in: Capsule())
        .overlay(Capsule().strokeBorder(Theme.Colors.hairline))
        .accessibilityElement(children: .combine)
        .accessibilityAction(named: Text(UndoCopy.undo)) { Task { await rt.undoRecordDelete() } }
        .accessibilityDismissAction(dismiss) // #628
        .swipeToDismiss(edge: .bottom, onDismiss: dismiss)
        .transition(.move(edge: .bottom).combined(with: .opacity))
        .task(id: undo.id) {
            try? await Task.sleep(for: .seconds(RecordUndo.seconds))
            guard !Task.isCancelled else { return }
            dismiss()
        }
    }
}
