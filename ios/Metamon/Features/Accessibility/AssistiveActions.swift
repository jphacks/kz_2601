import Foundation
import MetamonDSL

/// アシスティブアクセス(#1998)で開いた生成アプリに並べる大きなボタン。今の DSL から選ぶだけで、DSL・AI の決まりは変えない。
///
/// 選ぶ順: チェックの行(`list.item.toggle`)→ すぐ記録(`QuickRecord` で既定値だけで 1 件作れる entity)→ `stat` の数字。
/// どれも画面順(初期画面 → タブ順 → 残り)。1 画面 `limit` 個まで。共有(shared / public)の entity は選ばない
/// (他の人に見える記録をワンタップで作らない)。
enum AssistiveActions {
    static let limit = 4

    enum Kind: Equatable, Sendable {
        /// チェックの切り替え(`daily` は毎日リセットの date field)
        case check(entity: String, recordId: String, field: String, daily: Bool)
        /// 既定値だけで 1 件足す
        case quickRecord(entity: String)
        /// 数字を出すだけ(押せない)
        case stat(componentId: String)
    }

    struct Action: Equatable, Sendable, Identifiable {
        var kind: Kind
        /// ボタンの文字(記録の見出し・entity の名前・stat のラベル)。データなので訳さない
        var title: String
        /// チェック: 今付いているか
        var isOn: Bool = false
        /// stat: 表示する数字
        var value: String?

        var id: String {
            switch kind {
            case .check(let e, let r, let f, _): "check.\(e).\(r).\(f)"
            case .quickRecord(let e): "quick.\(e)"
            case .stat(let c): "stat.\(c)"
            }
        }
    }

    /// 簡単な画面を出すか(DSL の形だけで決める。記録の読み込みを待って画面が切り替わらないように)。
    /// 押せるボタン(チェック・すぐ記録)が無いアプリは今の画面をそのまま出す(#1997)。stat だけだと何もできない画面になるため。
    static func hasCandidates(in dsl: AppDSL) -> Bool {
        guard !dsl.isVoiceOnly else { return false }
        return !checkLists(in: dsl).isEmpty || !quickEntities(in: dsl).isEmpty
    }

    /// 読む entity(チェックの一覧と stat)。
    static func entities(in dsl: AppDSL) -> Set<String> {
        var out = Set(checkLists(in: dsl).compactMap(\.component.entity))
        for c in stats(in: dsl) { out.formUnion(AppGlance.entities(for: c, in: dsl)) }
        return out
    }

    /// 並べるボタン(最大 `limit` 個)。`records` は表示中の記録(未取得の entity は無いもの扱い)。
    static func pick(dsl: AppDSL, records: [String: [Record]], today: Date = DSLClock.now(), countUnit: String = "件") -> [Action] {
        guard !dsl.isVoiceOnly else { return [] }
        var out: [Action] = []

        for (c, field) in checkLists(in: dsl) {
            guard let entityId = c.entity, let entity = dsl.entity(entityId) else { continue }
            let daily = c.item?.resets == "daily"
            let rows = Aggregator.apply(c.filter, entity: entity, to: records[entityId] ?? [])
            let titleField = entity.displayTitleField
            for r in rows {
                let title = titleField.map { FieldCoding.display(r[$0], field: entity.field($0)) } ?? ""
                guard !title.isEmpty else { continue }
                let action = Action(kind: .check(entity: entityId, recordId: r.id, field: field.id, daily: daily),
                                    title: title, isOn: isOn(r, field: field.id, daily: daily, today: today))
                if !out.contains(where: { $0.id == action.id }) { out.append(action) }
                if out.count >= limit { return out }
            }
        }

        for e in quickEntities(in: dsl) {
            out.append(Action(kind: .quickRecord(entity: e.id), title: e.name))
            if out.count >= limit { return out }
        }

        let evaluator = ExprEvaluator(dsl: dsl, records: { records[$0] ?? [] })
        for c in stats(in: dsl) {
            let value = WidgetSnapshotWriter.statText(c, dsl: dsl, records: records, evaluator: evaluator, countUnit: countUnit)
            out.append(Action(kind: .stat(componentId: c.id), title: c.label ?? "", value: value ?? "—"))
            if out.count >= limit { return out }
        }
        return out
    }

    /// 押した時に保存する差分(チェック)・1 件分の data(すぐ記録)。stat は nil。
    /// チェックは表示中の状態(`action.isOn`・保存待ちの楽観的な値を含む)の逆を書く。保存前の `records` から決めると素早い 2 回押しで同じ値を書く。
    static func write(for action: Action, in dsl: AppDSL, records: [String: [Record]], today: Date = DSLClock.now()) -> [String: JSONValue]? {
        switch action.kind {
        case .check(let e, let rid, let field, let daily):
            guard let r = records[e]?.first(where: { $0.id == rid }) else { return nil }
            let next = !action.isOn
            return daily ? DailyCheck.patch(r, field: field, on: next, today: today) : [field: .bool(next)]
        case .quickRecord(let e):
            guard let entity = dsl.entity(e) else { return nil }
            let draft = QuickRecord.build(entity: entity, text: nil)
            return draft.missing.isEmpty ? draft.data : nil
        case .stat:
            return nil
        }
    }

    // MARK: - 候補

    /// 画面順(`WidgetSnapshotWriter.statComponents` と同じ)の部品。
    static func orderedComponents(in dsl: AppDSL) -> [Component] {
        var order = [dsl.initialScreen] + dsl.app.tabs.map(\.screen)
        order += dsl.screens.map(\.id).filter { !order.contains($0) }
        var seen = Set<String>()
        return order.filter { seen.insert($0).inserted }.compactMap { dsl.screen($0) }.flatMap(\.allComponents)
    }

    /// チェック丸のある一覧(toggle が boolean・daily なら date・computed でない)。
    static func checkLists(in dsl: AppDSL) -> [(component: Component, field: Field)] {
        orderedComponents(in: dsl).compactMap { c in
            guard c.type == .list, let id = c.item?.toggle, let entity = c.entity.flatMap(dsl.entity), !entity.isShared,
                  let f = entity.field(id), f.computed == nil,
                  f.type == (c.item?.resets == "daily" ? .date : .boolean) else { return nil }
            return (c, f)
        }
    }

    /// 既定値だけで 1 件作れる entity(フォーム画面があるもの・共有でない・チェックの一覧の entity でない)。
    static func quickEntities(in dsl: AppDSL) -> [Entity] {
        let checked = Set(checkLists(in: dsl).compactMap(\.component.entity))
        let withForm = Set(dsl.screens.compactMap(\.entity))
        return QuickRecord.recordableEntities(in: dsl).filter { e in
            withForm.contains(e.id) && !e.isShared && !checked.contains(e.id)
                && QuickRecord.build(entity: e, text: nil).missing.isEmpty
        }
    }

    static func stats(in dsl: AppDSL) -> [Component] {
        orderedComponents(in: dsl).filter { $0.type == .stat }
    }

    /// チェックが付いているか(一覧のチェック丸と同じ読み方)。
    static func isOn(_ r: Record, field: String, daily: Bool, today: Date) -> Bool {
        if daily { return DailyCheck.isDone(r[field], today: today) }
        switch r[field] {
        case .bool(let b): return b
        case .number(let n): return n != 0
        case .string(let s): return s == "true"
        default: return false
        }
    }
}
