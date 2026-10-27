import Foundation

/// `randomPicker`(#263)の選び方。描画(RandomPickerLeaf)から切り離し、乱数源を差し替えてテストする。
/// 乱数は Expr に入れない(描画のたびに評価されて値が変わる)。ボタンを押した時だけここを呼び、結果の id を部品の状態に持つ。
public enum RandomPick {
    /// 候補から 1 件。候補が 2 件以上なら `previous`(直前に選んだ id)は選ばない。候補が無ければ nil。
    public static func pick<G: RandomNumberGenerator>(from records: [Record], excluding previous: String?, using g: inout G) -> Record? {
        let pool = records.count > 1 ? records.filter { $0.id != previous } : records
        return pool.randomElement(using: &g)
    }

    public static func pick(from records: [Record], excluding previous: String?) -> Record? {
        var g = SystemRandomNumberGenerator()
        return pick(from: records, excluding: previous, using: &g)
    }

    /// シャッフル演出のコマ(当選の前に順に見せる候補)。隣り合うコマ・最後のコマと当選が同じにならないよう並べる。
    /// 候補が 1 件以下なら演出しない(空配列)。
    public static func reel<G: RandomNumberGenerator>(from records: [Record], ending winner: Record, frames: Int, using g: inout G) -> [Record] {
        guard records.count > 1, frames > 0 else { return [] }
        var out: [Record] = []
        var last = winner.id // 最後のコマ(= 当選の直前)が当選と同じだと止まって見える
        for _ in 0..<frames {
            let next = records.filter { $0.id != last }.randomElement(using: &g) ?? winner
            out.append(next)
            last = next.id
        }
        // 逆順にして「最後のコマ != 当選」を保証(生成は当選側から遡った)
        return out.reversed()
    }
}
