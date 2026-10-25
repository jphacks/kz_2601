import SwiftUI
import MetamonDSL

/// `gridTable`(#234): 時間割のマス目。横 = columnField(曜日など)、縦 = rowField(時限など)。
/// 配置は `GridTableLayout`。マスのタップ = `onItemTap`: 記録 1 件 → 編集、空 → 列・行の値を `FormPrefill` に積んで新規、
/// 複数 → メニューで選ぶ。列が画面幅に収まらない時だけ横スクロール(縦は GridCell が内側スクロール)。
struct GridTableLeaf: View {
    @Environment(\.dslRuntime) private var rt
    let c: Component
    let ctx: RenderContext
    @State private var width: CGFloat = 0

    private static let rowHeaderW: CGFloat = 30
    private static let minColumnW: CGFloat = 48
    private static let cellH: CGFloat = 58
    private static let gap: CGFloat = 4

    private var entityId: String { c.entity ?? "" }
    private var entity: Entity? { rt.dsl.entity(entityId) }

    var body: some View {
        let layout = GridTableLayout(component: c, entity: entity, records: rt.filteredRecords(c))
        let today = GridTableLayout.todayColumn(labels: layout.columns.map(\.label))
        VStack(alignment: .leading, spacing: RenderTheme.Spacing.sm) {
            if let title = c.title, !title.isEmpty {
                Text(title).font(RenderTheme.Fonts.sectionTitle).foregroundStyle(.secondary)
                    .accessibilityAddTraits(.isHeader)
            }
            ScrollView(.horizontal, showsIndicators: false) {
                grid(layout, today: today, columnW: columnWidth(count: layout.columns.count))
            }
            .scrollDisabled(fits(count: layout.columns.count))
            .onGeometryChange(for: CGFloat.self) { $0.size.width } action: { width = $0 }
            if case .unavailable = rt.emptyState(for: entityId) {
                HStack(spacing: RenderTheme.Spacing.sm) {
                    Text(EmptyPlaceholder.unavailableText).font(.caption).foregroundStyle(.secondary)
                    Button(EmptyPlaceholder.retryLabel) { Task { await rt.load(entities: [entityId], force: true) } }
                        .font(.caption.weight(.semibold))
                }
            }
        }
        .renderCard(padding: RenderTheme.Spacing.row)
    }

    // MARK: - sizing

    private func columnWidth(count: Int) -> CGFloat {
        guard count > 0, width > 0 else { return Self.minColumnW }
        let free = width - Self.rowHeaderW - Self.gap * CGFloat(count + 1) - 8 // 端の stroke・端数で最終列が欠けないよう余白を残す
        return max(Self.minColumnW, floor(free / CGFloat(count)))
    }

    private func fits(count: Int) -> Bool {
        Self.rowHeaderW + CGFloat(count) * (Self.minColumnW + Self.gap) + 8 <= width
    }

    // MARK: - grid

    private func grid(_ layout: GridTableLayout, today: Int?, columnW: CGFloat) -> some View {
        Grid(horizontalSpacing: Self.gap, verticalSpacing: Self.gap) {
            GridRow {
                Color.clear.frame(width: Self.rowHeaderW, height: 1)
                ForEach(Array(layout.columns.enumerated()), id: \.offset) { i, col in
                    columnHeader(col.label, isToday: i == today).frame(width: columnW)
                }
            }
            ForEach(Array(layout.rows.enumerated()), id: \.offset) { r, row in
                GridRow {
                    Text(row.label)
                        .font(RenderTheme.Fonts.tableHeader.monospacedDigit())
                        .foregroundStyle(.secondary)
                        .lineLimit(2)
                        .minimumScaleFactor(0.6)
                        .multilineTextAlignment(.center)
                        .frame(width: Self.rowHeaderW, height: Self.cellH)
                    ForEach(Array(layout.columns.enumerated()), id: \.offset) { ci, col in
                        let cell = GridTableLayout.Cell(column: ci, row: r)
                        cellView(layout, cell: cell, columnLabel: col.label, rowLabel: row.label, isToday: ci == today)
                            .frame(width: columnW, height: Self.cellH)
                    }
                }
            }
        }
    }

    private func columnHeader(_ label: String, isToday: Bool) -> some View {
        Text(label)
            .font(RenderTheme.Fonts.tableHeader)
            .fontWeight(isToday ? .bold : .semibold)
            .lineLimit(1)
            .minimumScaleFactor(0.6)
            .foregroundStyle(isToday ? Color.white : Color.secondary)
            .padding(.vertical, 4)
            .frame(maxWidth: .infinity)
            .background(isToday ? AnyShapeStyle(RenderTheme.brandFill) : AnyShapeStyle(.clear), in: Capsule())
            .accessibilityLabel(isToday ? label + L("、今日") : label)
    }

    @ViewBuilder
    private func cellView(_ layout: GridTableLayout, cell: GridTableLayout.Cell, columnLabel: String, rowLabel: String, isToday: Bool) -> some View {
        let rows = layout.records(at: cell)
        let place = "\(columnLabel) \(rowLabel)"
        if rows.count > 1, c.onItemTap != nil {
            Menu {
                ForEach(rows) { r in
                    Button(title(r)) { edit(r) }
                }
                Button { add(layout, cell) } label: { Label(L("追加"), systemImage: "plus") }
            } label: {
                filledCell(rows, isToday: isToday)
            }
            .buttonStyle(.plain)
            .tint(Color.primary) // Menu のラベルがアクセント色に染まらないように
            .accessibilityLabel(place)
            .accessibilityValue(rows.map(title).joined(separator: "、"))
            .accessibilityIdentifier("gridTable.cell.\(cell.column).\(cell.row)")
        } else {
            Button {
                if let r = rows.first { edit(r) } else { add(layout, cell) }
            } label: {
                if rows.isEmpty { emptyCell(isToday: isToday) } else { filledCell(rows, isToday: isToday) }
            }
            .buttonStyle(.plain)
            .disabled(c.onItemTap == nil)
            .accessibilityLabel(place)
            .accessibilityValue(rows.isEmpty ? L("空き") : rows.map(title).joined(separator: "、"))
            .accessibilityHint(rows.isEmpty ? L("タップで追加") : L("タップで編集"))
            .accessibilityIdentifier("gridTable.cell.\(cell.column).\(cell.row)")
        }
    }

    private func emptyCell(isToday: Bool) -> some View {
        let shape = RoundedRectangle(cornerRadius: RenderTheme.Radius.inner, style: .continuous)
        return shape
            .fill(isToday ? RenderTheme.Colors.brandSoft.opacity(0.6) : RenderTheme.Colors.canvas.opacity(0.6))
            .overlay(shape.strokeBorder(RenderTheme.Colors.hairline, style: StrokeStyle(lineWidth: 1, dash: [3, 3])))
            .contentShape(shape)
    }

    private func filledCell(_ rows: [Record], isToday: Bool) -> some View {
        let first = rows[0]
        let name = title(first)
        let tint = RenderTheme.Colors.category(name)
        let shape = RoundedRectangle(cornerRadius: RenderTheme.Radius.inner, style: .continuous)
        return VStack(alignment: .leading, spacing: 2) {
            Text(name)
                .font(.caption.weight(.semibold))
                .foregroundStyle(.primary)
                .lineLimit(2)
                .minimumScaleFactor(0.75)
            if let sub = subtitle(first) {
                Text(sub).font(.caption2).foregroundStyle(.secondary).lineLimit(1)
            }
            Spacer(minLength: 0)
        }
        .padding(.horizontal, 5)
        .padding(.vertical, 4)
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        .background(tint.opacity(0.18), in: shape)
        .overlay(shape.strokeBorder(isToday ? RenderTheme.Colors.brandA : tint.opacity(0.45), lineWidth: isToday ? 1.5 : 1))
        .overlay(alignment: .bottomTrailing) {
            if rows.count > 1 {
                Text("+\(rows.count - 1)")
                    .font(.caption2.weight(.bold).monospacedDigit())
                    .foregroundStyle(.white)
                    .padding(.horizontal, 4)
                    .padding(.vertical, 1)
                    .background(RenderTheme.Colors.brandA, in: Capsule())
                    .padding(3)
            }
        }
        .contentShape(shape)
    }

    // MARK: - values / actions

    private func title(_ r: Record) -> String {
        let f = c.titleField ?? entity?.displayTitleField ?? ""
        let s = rt.display(r[f], field: entity?.field(f))
        return s.isEmpty ? L("(無題)") : s
    }

    private func subtitle(_ r: Record) -> String? {
        guard let f = c.subtitleField, !f.isEmpty else { return nil }
        let s = rt.display(r[f], field: entity?.field(f))
        return s.isEmpty || s == FieldCoding.unavailable ? nil : s
    }

    private func edit(_ r: Record) {
        guard let action = c.onItemTap else { return }
        Task { await rt.run(action, ctx.actionContext(recordId: r.id)) }
    }

    /// 空のマス: 列・行の値を初期値として積み、onItemTap(通常はフォームへの navigate)を新規で実行する。
    private func add(_ layout: GridTableLayout, _ cell: GridTableLayout.Cell) {
        guard let action = c.onItemTap else { return }
        FormPrefill.set(layout.prefill(at: cell))
        Task { await rt.run(action, ctx.actionContext()) }
    }
}
