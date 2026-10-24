import Foundation

/// 精算(割り勘の「誰が誰にいくら払うか」)。balance は 正 = 受け取る / 負 = 払う(#101 `settlement` leaf)。
public enum Settlement {
    public struct Transfer: Hashable, Sendable {
        public var from: String
        public var to: String
        public var amount: Double
    }

    /// 貪欲法: 最大債務者 → 最大債権者へ min(|債務|, 債権) を移す、を繰り返す。
    /// 端数は `unit`(既定 1 = 円)未満を 0 とみなす。合計が 0 にならない入力でも、移せる分だけ返す。
    public static func transfers(_ balances: [(name: String, balance: Double)], unit: Double = 1) -> [Transfer] {
        let eps = unit / 2
        var debtors = balances.filter { $0.balance < -eps }.map { (name: $0.name, amount: -$0.balance) }
        var creditors = balances.filter { $0.balance > eps }.map { (name: $0.name, amount: $0.balance) }
        var out: [Transfer] = []
        while true {
            debtors.sort { $0.amount > $1.amount }
            creditors.sort { $0.amount > $1.amount }
            guard let d = debtors.first, let c = creditors.first, d.amount > eps, c.amount > eps else { break }
            let x = Swift.min(d.amount, c.amount)
            let rounded = (x / unit).rounded() * unit
            if rounded > 0 { out.append(Transfer(from: d.name, to: c.name, amount: rounded)) }
            debtors[0].amount -= x
            creditors[0].amount -= x
            debtors.removeAll { $0.amount <= eps }
            creditors.removeAll { $0.amount <= eps }
        }
        return out
    }
}
