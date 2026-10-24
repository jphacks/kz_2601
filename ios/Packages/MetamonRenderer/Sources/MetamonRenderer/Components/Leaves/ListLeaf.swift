import SwiftUI
import MetamonDSL

/// `list`: entity のレコード一覧(filter/sort はクライアント側)。行タップで `onItemTap`、長押しで削除(swipeToDelete)。
struct ListLeaf: View {
    @Environment(\.dslRuntime) private var rt
    let c: Component
    let ctx: RenderContext

    var body: some View {
        let entityId = c.entity ?? ""
        let entity = rt.dsl.entity(entityId)
        let filtered = Aggregator.sorted(Aggregator.apply(c.filter, entity: entity, to: rt.records(for: entityId)), by: c.sort, entity: entity)
        let rows = searched(filtered, entity: entity)
        if rows.isEmpty {
            EmptyPlaceholder(icon: "tray", emptyText: c.emptyText ?? "まだありません", state: rt.emptyState(for: entityId)) {
                Task { await rt.load(entities: [entityId], force: true) }
            }
        } else {
            // 1 枚のカードに行を積み、行間は 1pt の hairline(iOS の inset grouped list と同じ読み味)
            LazyVStack(spacing: 0) {
                ForEach(Array(rows.enumerated()), id: \.element.id) { index, row in
                    if index > 0 { Hairline(inset: separatorInset) }
                    ListRow(item: c.item, record: row, entity: entity, showsChevron: c.onItemTap != nil)
                        .contentShape(Rectangle())
                        .onTapGesture { open(row) }
                        // VoiceOver: onTapGesture は読まれないので、行をボタンとして宣言しダブルタップで同じ遷移を実行
                        .accessibilityAddTraits(c.onItemTap != nil ? .isButton : [])
                        .accessibilityHint(c.onItemTap != nil ? "ダブルタップで開きます" : "")
                        .accessibilityAction { open(row) }
                        .contextMenu {
                            if c.swipeToDelete == true {
                                Button("削除", systemImage: "trash", role: .destructive) {
                                    Task { await rt.run(Action(type: .deleteRecord, entity: entityId), ctx.actionContext(recordId: row.id)) }
                                }
                            }
                        }
                }
            }
            // 行が少なくてもカードはセルの高さまで伸ばす(#84: 空白の下半分を作らない)。行は上詰め
            .frame(maxHeight: .infinity, alignment: .top)
            .renderCard(padding: 0)
        }
    }

    private func open(_ row: Record) {
        guard let action = c.onItemTap else { return }
        Task { await rt.run(action, ctx.actionContext(recordId: row.id)) }
    }

    /// 先頭のアイコン/画像の幅ぶんセパレータを内側に寄せる。
    private var separatorInset: CGFloat {
        if c.item?.image != nil { return RenderTheme.Spacing.md + 52 + RenderTheme.Spacing.row }
        if c.item?.icon != nil { return RenderTheme.Spacing.md + 32 + RenderTheme.Spacing.row }
        return RenderTheme.Spacing.md
    }

    /// `searchField`(同画面の searchBar.field)の文字列で title/subtitle を部分一致(大小無視)フィルタ。
    private func searched(_ rows: [Record], entity: Entity?) -> [Record] {
        guard let key = c.searchField, let q = ctx.locals?.values[key]?.trimmingCharacters(in: .whitespaces), !q.isEmpty else { return rows }
        let fields = [c.item?.title ?? entity?.titleField, c.item?.subtitle].compactMap { $0 }
        return rows.filter { r in
            fields.contains { FieldCoding.display(r[$0], field: entity?.field($0)).localizedCaseInsensitiveContains(q) }
        }
    }
}
