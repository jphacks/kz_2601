import SwiftUI
import MetamonServices

/// 記録の同期の状態(#976)。iOS の設定の「同期」(#48)と同じ文言を、送り切っていない時だけツールバーに出す。
/// 押すと今すぐ送り直す(オフラインの時は押せない)。
struct MacSyncStatusToolbar: ToolbarContent {
    let engine: RecordSyncEngine

    var body: some ToolbarContent {
        ToolbarItem(placement: .status) {
            if engine.status != .synced {
                Button {
                    Task { await MacRecordResend.shared.resend(engine) }
                } label: {
                    Label(Self.label(engine.status), systemImage: Self.symbol(engine.status))
                        .labelStyle(.titleAndIcon)
                }
                .disabled(!engine.isOnline)
                .help(engine.isOnline ? L10n.tr("今すぐ同期") : L10n.tr("つながったら送ります"))
                .accessibilityIdentifier("syncStatus.button")
            }
        }
    }

    /// iOS の `SettingsSheet.syncLabel` と同じ(表示言語対応)
    static func label(_ status: RecordSyncEngine.Status) -> String {
        switch status {
        case .synced: L10n.tr("最新")
        case .pending(let n): L10n.tr("未送信 \(n) 件")
        case .syncing(let n): L10n.tr("送信中(\(n) 件)")
        case .offline(let n): n == 0 ? L10n.tr("オフライン") : L10n.tr("オフライン・未送信 \(n) 件")
        }
    }

    static func symbol(_ status: RecordSyncEngine.Status) -> String {
        switch status {
        case .synced: "checkmark.icloud"
        case .pending, .syncing: "arrow.triangle.2.circlepath.icloud"
        case .offline: "icloud.slash"
        }
    }
}
