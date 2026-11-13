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
        let cells = Self.cellRows(c, rt: rt)
        #if os(macOS)
        if MacTableExperiment.isEnabled(), !cells.isEmpty {
            if #available(macOS 14.4, *) {
                // 試作(#2326・既定 off): Mac の Table で描く。集計・並びは今と同じ `cellRows`
                MacTableLeafTable(columns: columns.enumerated().map { i, col in
                    MacTableColumn(index: i, title: Self.header(for: col, entity: entity), field: col.field,
                                   numeric: isNumericColumn(col, entity: entity))
                }, rows: cells)
                .renderCard()
            } else {
                standard(columns: columns, cells: cells, entity: entity, entityId: entityId)
            }
        } else {
            standard(columns: columns, cells: cells, entity: entity, entityId: entityId)
        }
        #else
        standard(columns: columns, cells: cells, entity: entity, entityId: entityId)
        #endif
    }

    /// 今の描き方(見出し行 + 行×列の Grid)
    private func standard(columns: [TableColumnSpec], cells: [CellRow], entity: Entity?, entityId: String) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            Grid(alignment: .leading, horizontalSpacing: RenderTheme.Spacing.row, verticalSpacing: 10) {
                GridRow {
                    ForEach(Array(columns.enumerated()), id: \.offset) { _, col in
                        Text(Self.header(for: col, entity: entity))
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

    struct CellRow: Identifiable {
        var id: String
        var texts: [String]
        /// 列の見出しで並べ替える値(Mac の Table の試作・#2326)
        var keys: [TableSortKey] = []
    }

    /// 表示する行(列ごとの文字列)。グループ表なら集計済みの行、それ以外はレコード 1 件 = 1 行。
    /// 適合テストの描画の木(`RenderTree`)も同じ行を使う。
    @MainActor
    static func cellRows(_ c: Component, rt: any DSLRuntime) -> [CellRow] {
        let entity = rt.dsl.entity(c.entity ?? "")
        let columns = c.columns ?? []
        if TableGrouping.isGrouped(c) {
            return TableGrouping.rows(c, records: rt.filteredRecords(c)).map { row in
                let texts = columns.enumerated().map { i, col in
                    col.aggregate == nil
                        ? rt.display(row.sample[col.field], field: entity?.field(col.field))
                        : aggregateText(row.values[i], op: col.aggregate, field: entity?.field(col.field), rt: rt)
                }
                return CellRow(id: row.key, texts: texts, keys: columns.enumerated().map { i, col in
                    col.aggregate == nil ? TableSortKey.of(row.sample[col.field], display: texts[i])
                        : row.values[i].map(TableSortKey.number) ?? .none
                })
            }
        }
        let all = Aggregator.sorted(rt.filteredRecords(c), by: c.sort, entity: entity)
        let rows = (c.limit ?? 0) > 0 ? Array(all.prefix(c.limit!)) : all
        return rows.map { row in
            let texts = columns.map { rt.display(row[$0.field], field: entity?.field($0.field)) }
            return CellRow(id: row.id, texts: texts,
                           keys: columns.enumerated().map { i, col in TableSortKey.of(row[col.field], display: texts[i]) })
        }
    }

    /// count は件数、sum/min/max は field の書式(¥・単位)、avg は小数 1 桁(currency は円の整数)。
    @MainActor
    private static func aggregateText(_ value: Double?, op: String?, field: Field?, rt: any DSLRuntime) -> String {
        guard let value else { return FieldCoding.unavailable }
        if op == "count" { return FieldCoding.plainNumber(value) }
        let shown = op == "avg" && field?.type != .currency ? (value * 10).rounded() / 10 : value
        guard let field else { return FieldCoding.plainNumber(shown) }
        return rt.display(.number(shown), field: field)
    }

    static func header(for col: TableColumnSpec, entity: Entity?) -> String {
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

#if os(macOS)
/// Mac の Table で描く table(#2326 の試作)。列の見出しで並べ替え・列のドラッグで並べ替え・境目で幅を変える・行を選ぶ・右クリックでコピー
@available(macOS 14.4, *)
struct MacTableLeafTable: View {
    let columns: [MacTableColumn]
    let rows: [TableLeaf.CellRow]
    @State private var sortOrder: [MacTableSort] = []
    @State private var selection = Set<String>()
    @State private var customization = TableColumnCustomization<TableLeaf.CellRow>()

    var body: some View {
        Table(MacTableSort.sorted(rows, by: sortOrder), selection: $selection, sortOrder: $sortOrder,
              columnCustomization: $customization) {
            TableColumnForEach(columns) { col in
                TableColumn(col.title, sortUsing: MacTableSort(column: col.index)) { row in
                    Text(col.index < row.texts.count ? row.texts[col.index] : "")
                        .font(RenderTheme.Fonts.tableCell)
                        .lineLimit(1)
                        .frame(maxWidth: .infinity, alignment: col.numeric ? .trailing : .leading)
                }
                .customizationID(col.id)
            }
        }
        .contextMenu(forSelectionType: String.self) { ids in
            Button(L("コピー")) {
                PlatformPasteboard.setString(MacTableCopy.text(columns: columns, rows: rows.filter { ids.contains($0.id) }))
            }
            .disabled(ids.isEmpty)
        }
        .frame(minHeight: min(CGFloat(rows.count + 1) * 26 + 8, 360))
        .accessibilityIdentifier("table.macTable")
    }
}
#endif
