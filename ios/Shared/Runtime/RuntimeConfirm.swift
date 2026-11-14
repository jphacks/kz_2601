import Foundation
import MetamonDSL

/// アクションの `confirm` で出す確認ダイアログの中身(iOS の alert・Mac の confirmationDialog が同じものを読む)。
struct RuntimeConfirm: Identifiable {
    let id = UUID()
    var message: String
    /// 主ボタンの文言(L1: 「実行」でなく何が起きるか)と、取り消せない操作か
    var primaryLabel: String = L10n.tr("続ける")
    var isDestructive = false
    var perform: @MainActor () async -> Void

    /// アクションの種類から主ボタンを決める(L1)
    static func button(for type: ActionType) -> (label: String, destructive: Bool) {
        switch type {
        case .deleteRecord: (L10n.tr("削除"), true)
        case .resetForm: (L10n.tr("入力を消す"), true)
        case .saveRecord: (L10n.tr("保存"), false)
        case .export: (L10n.tr("書き出す"), false)
        case .navigate, .openUrl: (L10n.tr("開く"), false)
        default: (L10n.tr("続ける"), false)
        }
    }
}
