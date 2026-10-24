import Foundation
import MetamonDSL

/// `list.item.toggle`(#108)の判定と楽観的更新の状態。View から切り離してテストする。
/// - 行頭のチェック丸は「保存される boolean field」を指す時だけ出す(壊れた DSL では出さない = 押しても保存できない丸を見せない)。
/// - 楽観的 UI: タップ直後に `overrides[recordId]` へ新しい値を入れて描画 → 保存の結果が返ったら外す。
///   成功ならストアの値が新しい値になっているので見た目は変わらず、失敗ならストアの古い値に戻る。
enum ListToggle {
    /// チェック丸に使う field(boolean・computed でない)。条件を満たさなければ nil。
    static func field(_ item: ListItem?, in entity: Entity?) -> Field? {
        guard let id = item?.toggle, let f = entity?.field(id), f.type == .boolean, f.computed == nil else { return nil }
        return f
    }

    /// 表示上の ON/OFF(保存待ちの値を優先)。bool 以外の保存値("true" / 1)も寛容に読む。
    static func isOn(_ record: Record, field: String, overrides: [String: Bool]) -> Bool {
        if let pending = overrides[record.id] { return pending }
        switch record[field] {
        case .bool(let b): return b
        case .number(let n): return n != 0
        case .string(let s): return s == "true"
        default: return false
        }
    }

    /// 保存の結果が返った時に呼ぶ。その後に別の値でタップされていたら(連打)その表示は残す。
    static func settle(_ overrides: inout [String: Bool], recordId: String, requested: Bool) {
        if overrides[recordId] == requested { overrides[recordId] = nil }
    }
}
