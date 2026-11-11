import Foundation
import MetamonServices

/// 置き換えシート・状態カードの文言(ja・en は String Catalog)。Mac の状態カードと共有する。
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
