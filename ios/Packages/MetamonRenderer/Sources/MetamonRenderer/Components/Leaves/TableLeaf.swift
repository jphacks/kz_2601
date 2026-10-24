import SwiftUI
import MetamonDSL

/// `table`: entity のレコードを 見出し行 + 行×列 の Grid で(filter/sort/limit はクライアント側・list と同じ経路)。
struct TableLeaf: View {
    @Environment(\.dslRuntime) private var rt
    let c: Component

    var body: some View {
        let entityId = c.entity ?? ""
        let entity = rt.dsl.entity(entityId)
        let columns = c.columns ?? []
        let all = Aggregator.sorted(Aggregator.apply(c.filter, entity: entity, to: rt.records(for: entityId)), by: c.sort, entity: entity)
        let rows = (c.limit ?? 0) > 0 ? Array(all.prefix(c.limit!)) : all
        VStack(alignment: .leading, spacing: 0) {
            Grid(alignment: .leading, horizontalSpacing: RenderTheme.Spacing.row, verticalSpacing: 10) {
                GridRow {
                    ForEach(Array(columns.enumerated()), id: \.offset) { _, col in
                        Text(col.title ?? entity?.field(col.field)?.name ?? col.field)
                            .font(RenderTheme.Fonts.tableHeader)
                            .foregroundStyle(.secondary)
                            .gridColumnAlignment(alignment(for: col, entity: entity))
                    }
                }
                Hairline().gridCellUnsizedAxes(.horizontal)
                ForEach(rows) { row in
                    GridRow {
                        ForEach(Array(columns.enumerated()), id: \.offset) { _, col in
                            Text(FieldCoding.display(row[col.field], field: entity?.field(col.field)))
                                .font(RenderTheme.Fonts.tableCell)
                                .lineLimit(1)
                                .frame(maxWidth: .infinity, alignment: frameAlignment(for: col, entity: entity))
                        }
                    }
                }
            }
            if rows.isEmpty {
                EmptyPlaceholder(icon: "tablecells", emptyText: "まだありません", state: rt.emptyState(for: entityId)) {
                    Task { await rt.load(entities: [entityId], force: true) }
                }
                    .padding(.top, RenderTheme.Spacing.sm)
            }
        }
        .renderCard()
    }

    private func alignment(for col: TableColumnSpec, entity: Entity?) -> HorizontalAlignment {
        entity?.field(col.field)?.type.isNumeric == true ? .trailing : .leading
    }

    private func frameAlignment(for col: TableColumnSpec, entity: Entity?) -> Alignment {
        entity?.field(col.field)?.type.isNumeric == true ? .trailing : .leading
    }
}
