#if !APP_STORE_REVIEW // 審査用ビルドはコインを持たない(#450)
import Foundation
import MetamonServices

/// コイン(#630)の文言。数はすべてサーバーから(アプリに値段・枚数を持たない)。
enum CoinCopy {
    static func coins(_ n: Int) -> String { L10n.tr("\(n) コイン") }

    /// 押す前の目安
    static func estimate(_ n: Int) -> String { L10n.tr("約 \(n) コイン") }

    static func estimateAccessibility(_ n: Int, balance: Int?) -> String {
        guard let balance else { return L10n.tr("送ると約 \(n) コインを使います") }
        return L10n.tr("送ると約 \(n) コインを使います。残り \(balance) コイン")
    }

    static var balanceTitle: String { L10n.tr("コインの残高") }
    static var purchasedTitle: String { L10n.tr("買ったコイン(期限なし)") }
    static var initialTitle: String { L10n.tr("最初のコイン") }
    static var initialGrantTitle: String { L10n.tr("最初のコイン(1 回だけ)") }

    /// Pro の毎月の分(3.1.1: 買った分と分けて「その月の分」と書く)
    static func grantTitle(until: Date?) -> String {
        guard let until else { return L10n.tr("今月の Pro の分(繰り越しません)") }
        return L10n.tr("Pro の分(\(until.formatted(date: .abbreviated, time: .omitted)) まで・繰り越しません)")
    }

    // MARK: - 断り(402)

    static func denialTitle(_ kind: CoinDenial.Kind) -> String {
        switch kind {
        case .coinsInsufficient: L10n.tr("コインが足りません")
        case .allowanceExhausted: L10n.tr("今月の枠を使い切りました")
        case .overflowCapReached: L10n.tr("今月のコインの上限に着きました")
        }
    }

    static func denialBody(_ d: CoinDenial) -> String {
        if !d.payerIsSelf {
            switch d.kind {
            case .coinsInsufficient: return L10n.tr("このアプリは作った人のコインで動いています。作った人のコインが足りないので、今は使えません。")
            case .allowanceExhausted, .overflowCapReached: return L10n.tr("このアプリは作った人の枠で動いています。作った人の今月の枠が終わったので、今は使えません。")
            }
        }
        switch d.kind {
        case .coinsInsufficient:
            if let required = d.required, let balance = d.balance {
                return L10n.tr("約 \(required) コイン要りますが、残りは \(balance) コインです。コインを足すと続けられます。")
            }
            return L10n.tr("コインを足すと続けられます。")
        case .allowanceExhausted:
            return L10n.tr("今月のサーバーの枠を使い切ったので止めました。「枠を超えたらコインで続ける」を ON にすると、決めた上限までコインで続けられます。")
        case .overflowCapReached:
            return L10n.tr("決めた月の上限までコインを使ったので止めました。上限は設定で変えられます。")
        }
    }

    // MARK: - 枠・プラン

    /// 枠の名前。知らない種類は nil(出さない)
    static func allowanceName(_ key: String) -> String? {
        switch key {
        case "runs": L10n.tr("自動の動き")
        case "recipients": L10n.tr("通知の宛先")
        case "notifications": L10n.tr("通知")
        case "writes": L10n.tr("記録の書き込み")
        case "reads": L10n.tr("記録の読み取り")
        default: nil
        }
    }

    /// 枠の表示順(サーバーの辞書は順が無い)
    static func orderedAllowanceKeys(_ keys: some Collection<String>) -> [String] {
        let order = ["runs", "recipients", "notifications", "writes", "reads"]
        return keys.filter { allowanceName($0) != nil }.sorted { (order.firstIndex(of: $0) ?? 99) < (order.firstIndex(of: $1) ?? 99) }
    }

    static func perMonth(_ n: Int) -> String { L10n.tr("月 \(n) 回") }

    static func meter(_ m: UsageMeter) -> String {
        guard let limit = m.limit else { return L10n.tr("\(m.used) 回") }
        return L10n.tr("\(m.used) / \(limit) 回")
    }

    // MARK: - 履歴

    static func ledgerKind(_ kind: String) -> String {
        switch kind {
        case "purchase": L10n.tr("購入")
        case "grant_initial": L10n.tr("最初のコイン")
        case "grant_monthly": L10n.tr("Pro の毎月の分")
        case "grant_expire": L10n.tr("月の分の終わり")
        case "spend_ai": L10n.tr("使った")
        case "spend_overflow": L10n.tr("枠を超えた分")
        case "spend_slot": L10n.tr("アプリの枠")
        case "spend_call": L10n.tr("通話")
        case "refund_failed": L10n.tr("戻した(失敗)")
        case "refund_apple": L10n.tr("返金")
        case "adjust": L10n.tr("調整")
        default: kind
        }
    }

    static func operationName(_ op: String?) -> String? {
        switch op.map(CoinEstimates.normalizedKey) ?? "" {
        case "generate", "create": L10n.tr("新しく作る")
        case "edit": L10n.tr("直す")
        case "chat": L10n.tr("相談")
        case "text", "aibutton", "aichat": L10n.tr("アプリの中の AI")
        default: nil
        }
    }

    static func signedDelta(_ n: Int) -> String { n > 0 ? "+\(n)" : "\(n)" }
}

extension CoinEstimates {
    static func normalizedKey(_ key: String) -> String {
        key.lowercased().replacingOccurrences(of: "_", with: "").replacingOccurrences(of: "-", with: "")
    }
}
#endif
