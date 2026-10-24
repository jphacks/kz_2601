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
        let rows = Aggregator.sorted(Aggregator.apply(c.filter, entity: entity, to: rt.records(for: entityId)), by: c.sort, entity: entity)
        if rows.isEmpty {
            Text(rt.loading.contains(entityId) ? "読み込み中…" : (c.emptyText ?? "まだありません"))
                .foregroundStyle(.secondary)
                .frame(maxWidth: .infinity)
                .padding(.vertical, 24)
        } else {
            LazyVStack(spacing: 8) {
                ForEach(rows) { row in
                    ListRow(item: c.item, record: row, entity: entity)
                        .contentShape(Rectangle())
                        .onTapGesture {
                            guard let action = c.onItemTap else { return }
                            Task { await rt.run(action, ctx.actionContext(recordId: row.id)) }
                        }
                        .contextMenu {
                            if c.swipeToDelete == true {
                                Button("削除", systemImage: "trash", role: .destructive) {
                                    Task { await rt.run(Action(type: .deleteRecord, entity: entityId), ctx.actionContext(recordId: row.id)) }
                                }
                            }
                        }
                }
            }
        }
    }
}
