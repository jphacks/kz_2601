import Foundation
import MetamonDSL

/// `list.item.toggle`(#108)の判定と楽観的更新の状態。View から切り離してテストする。
/// - 行頭のチェック丸は「保存される boolean field」を指す時だけ出す(壊れた DSL では出さない = 押しても保存できない丸を見せない)。
/// - 楽観的 UI: タップ直後に `overrides[recordId]` へ新しい値を入れて描画 → 保存の結果が返ったら外す。
///   成功ならストアの値が新しい値になっているので見た目は変わらず、失敗ならストアの古い値に戻る。
enum ListToggle {
    /// チェック丸に使う field(boolean・computed でない。`resets: "daily"` なら date field)。条件を満たさなければ nil。
    static func field(_ item: ListItem?, in entity: Entity?) -> Field? {
        guard let id = item?.toggle, let f = entity?.field(id), f.computed == nil else { return nil }
        return f.type == (isDaily(item) ? .date : .boolean) ? f : nil
    }

    /// 毎日リセットのチェック(#132): 値が今日の日付なら済み。日付が変わると未チェックに戻る。
    static func isDaily(_ item: ListItem?) -> Bool { item?.resets == "daily" }

    /// 表示上の ON/OFF。daily は「保存値 == 今日」、それ以外は boolean。
    static func isOn(_ record: Record, field: String, daily: Bool, overrides: [String: Bool], today: Date) -> Bool {
        if let pending = overrides[record.id] { return pending }
        return daily ? DailyCheck.isDone(record[field], today: today) : isOn(record, field: field, overrides: [:])
    }

    /// 保存する値(daily は今日の日付 / null、それ以外は bool)。
    static func stored(_ on: Bool, daily: Bool, today: Date) -> JSONValue {
        daily ? DailyCheck.stored(on, today: today) : .bool(on)
    }

    /// 保存する差分。daily は「最後にやった日」+ 押した日の履歴(#172: 付けたら今日を足し、外したら今日を消す)、それ以外は bool 1 つ。
    static func patch(_ record: Record, field: String, on: Bool, daily: Bool, today: Date) -> [String: JSONValue] {
        daily ? DailyCheck.patch(record, field: field, on: on, today: today) : [field: .bool(on)]
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
