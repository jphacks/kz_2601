import SwiftUI
import MetamonDSL

/// `table`: entity のレコードを 見出し行 + 行×列 の Grid で(filter/sort/limit はクライアント側・list と同じ経路)。
/// `groupBy` + 集計列があれば 1 グループ = 1 行の集計表(#261・`TableGrouping`)。
struct TableLeaf: View {
    @Environment(\.dslRuntime) private var rt
    let c: Component

    var body: some View {
        let entityId = c.entity ?? ""
        let entity = rt.dsl.entity(entityId)
        let columns = c.columns ?? []
        let cells = cellRows(entity: entity, columns: columns)
        VStack(alignment: .leading, spacing: 0) {
            Grid(alignment: .leading, horizontalSpacing: RenderTheme.Spacing.row, verticalSpacing: 10) {
                GridRow {
                    ForEach(Array(columns.enumerated()), id: \.offset) { _, col in
                        Text(header(for: col, entity: entity))
                            .font(RenderTheme.Fonts.tableHeader)
                            .foregroundStyle(.secondary)
                            .gridColumnAlignment(alignment(for: col, entity: entity))
                    }
                }
                Hairline().gridCellUnsizedAxes(.horizontal)
                ForEach(cells, id: \.id) { row in
                    GridRow {
                        ForEach(Array(columns.enumerated()), id: \.offset) { i, col in
                            Text(row.texts[i])
                                .font(RenderTheme.Fonts.tableCell)
                                .lineLimit(1)
                                .frame(maxWidth: .infinity, alignment: frameAlignment(for: col, entity: entity))
                        }
                    }
                }
            }
            if cells.isEmpty {
                EmptyPlaceholder(icon: "tablecells", emptyText: L("まだありません"), state: rt.emptyState(for: entityId)) {
                    Task { await rt.load(entities: [entityId], force: true) }
                }
                    .padding(.top, RenderTheme.Spacing.sm)
            }
        }
        .renderCard()
    }

    private struct CellRow {
        var id: String
        var texts: [String]
    }

    /// 表示する行(列ごとの文字列)。グループ表なら集計済みの行、それ以外はレコード 1 件 = 1 行。
    private func cellRows(entity: Entity?, columns: [TableColumnSpec]) -> [CellRow] {
        if TableGrouping.isGrouped(c) {
            return TableGrouping.rows(c, records: rt.filteredRecords(c)).map { row in
                CellRow(id: row.key, texts: columns.enumerated().map { i, col in
                    col.aggregate == nil
                        ? rt.display(row.sample[col.field], field: entity?.field(col.field))
                        : aggregateText(row.values[i], op: col.aggregate, field: entity?.field(col.field))
                })
            }
        }
        let all = Aggregator.sorted(rt.filteredRecords(c), by: c.sort, entity: entity)
        let rows = (c.limit ?? 0) > 0 ? Array(all.prefix(c.limit!)) : all
        return rows.map { row in
            CellRow(id: row.id, texts: columns.map { rt.display(row[$0.field], field: entity?.field($0.field)) })
        }
    }

    /// count は件数、sum/min/max は field の書式(¥・単位)、avg は小数 1 桁(currency は円の整数)。
    private func aggregateText(_ value: Double?, op: String?, field: Field?) -> String {
        guard let value else { return FieldCoding.unavailable }
        if op == "count" { return FieldCoding.plainNumber(value) }
        let shown = op == "avg" && field?.type != .currency ? (value * 10).rounded() / 10 : value
        guard let field else { return FieldCoding.plainNumber(shown) }
        return rt.display(.number(shown), field: field)
    }

    private func header(for col: TableColumnSpec, entity: Entity?) -> String {
        if let title = col.title { return title }
        let name = entity?.field(col.field)?.name ?? col.field
        switch col.aggregate {
        case "count": return L("件数")
        case "sum": return L("\(name)の合計")
        case "avg": return L("\(name)の平均")
        case "min": return L("\(name)の最小")
        case "max": return L("\(name)の最大")
        default: return name
        }
    }

    private func isNumericColumn(_ col: TableColumnSpec, entity: Entity?) -> Bool {
        col.aggregate != nil || entity?.field(col.field)?.type.isNumeric == true
    }

    private func alignment(for col: TableColumnSpec, entity: Entity?) -> HorizontalAlignment {
        isNumericColumn(col, entity: entity) ? .trailing : .leading
    }

    private func frameAlignment(for col: TableColumnSpec, entity: Entity?) -> Alignment {
        isNumericColumn(col, entity: entity) ? .trailing : .leading
    }
}
