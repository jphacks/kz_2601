import Foundation
import MetamonServices

/// 「上限に達しています。どのアプリと置き換えますか?」(#630・402 `app_limit_reached`)の中身。純粋関数だけ(画面は `AppLimitSheet`)。
/// 並べるのはサーバーが返した持っているアプリ(無ければ端末の一覧)。長く開いていないものから並べ、使用中のアプリは最後。
/// 自分で作ったアプリは削除、参加した共有のアプリは抜ける。枠を買う道は、サーバーが買えると言い、コインの画面が有効な時だけ。
struct AppReplacePlan: Equatable {
    struct Row: Equatable, Identifiable {
        enum Role: Equatable {
            /// 自分で作った → 削除(データも消える)
            case own
            /// 参加した共有のアプリ → 抜ける
            case joined
        }

        var id: String
        var name: String
        var role: Role
        /// 端末で最後に開いた時刻(無ければサーバーの更新日時)
        var lastOpened: Date?
        var isCurrent: Bool
    }

    enum Action: Equatable {
        case delete(appId: String)
        case leave(appId: String)
    }

    /// 枠を 1 つ買う道
    struct SlotOffer: Equatable {
        var coins: Int
        /// 残高で足りるか(分からなければ true。足りなければ押した時にお店へ)
        var affordable: Bool
    }

    var rows: [Row]
    var slot: SlotOffer?
    var held: Int
    var limit: Int

    /// あと何個消せば 1 つ入るか(上限を下げた後は 2 つ以上のこともある)
    var removalsNeeded: Int { max(1, held - limit + 1) }

    static func make(_ limit: AppLimit, local: [AppSummary], lastOpened: (String) -> Date?, current: String?,
                     coinsEnabled: Bool) -> AppReplacePlan {
        let localByID = Dictionary(local.map { ($0.id, $0) }, uniquingKeysWith: { a, _ in a })
        var rows: [Row]
        if let held = limit.apps, !held.isEmpty {
            rows = held.map { app in
                let name = app.name.isEmpty ? (localByID[app.id]?.name ?? "") : app.name
                return Row(id: app.id, name: name, role: app.mine ? .own : .joined,
                           lastOpened: lastOpened(app.id) ?? app.updatedAt ?? localByID[app.id]?.updatedDate,
                           isCurrent: app.id == current)
            }
        } else {
            // サーバーが一覧を返さなかった: 端末の一覧(空のアプリ・アーカイブは数えないので除く)。持ち主は分からないので削除として扱う
            // (`DELETE /v1/apps/{id}` は参加したアプリなら自分の写しだけを消す)
            rows = local.filter { !$0.isArchived && !$0.isBlank }.map { app in
                Row(id: app.id, name: app.name, role: .own, lastOpened: lastOpened(app.id) ?? app.updatedDate,
                    isCurrent: app.id == current)
            }
        }
        rows.sort { a, b in
            if a.isCurrent != b.isCurrent { return !a.isCurrent }
            switch (a.lastOpened, b.lastOpened) {
            case let (x?, y?): return x != y ? x < y : a.id < b.id
            case (nil, _?): return true
            case (_?, nil): return false
            case (nil, nil): return a.id < b.id
            }
        }
        var slot: SlotOffer?
        if coinsEnabled, limit.canBuySlot, let coins = limit.slotCoins, coins > 0 {
            slot = SlotOffer(coins: coins, affordable: limit.balance.map { $0 >= coins } ?? true)
        }
        return AppReplacePlan(rows: rows, slot: slot, held: limit.held, limit: limit.limit)
    }

    static func action(for row: Row) -> Action {
        switch row.role {
        case .own: .delete(appId: row.id)
        case .joined: .leave(appId: row.id)
        }
    }

    /// `row` を消した / 抜けた後の計画。まだ足りなければ残りを並べ直す(nil = もう 1 つ入る・元の操作を続ける)。
    func removing(_ id: String) -> AppReplacePlan? {
        guard rows.contains(where: { $0.id == id }) else { return self }
        let next = AppReplacePlan(rows: rows.filter { $0.id != id }, slot: slot, held: max(0, held - 1), limit: limit)
        return next.held < next.limit ? nil : next
    }

    /// 枠を買った後(nil = もう 1 つ入る)
    func addingSlot() -> AppReplacePlan? {
        let next = AppReplacePlan(rows: rows, slot: slot, held: held, limit: limit + 1)
        return next.held < next.limit ? nil : next
    }
}
