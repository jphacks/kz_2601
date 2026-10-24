import SwiftUI
import MetamonDSL

/// `calendar`: 月カレンダー(自作)。`entity.dateField` でレコードを日に載せ、レコードのある日に点を打つ。
/// 日をタップ → その日のレコード一覧を下に表示。行タップ = `onDateTap` に recordId を渡す(編集)、
/// 「+」= `onDateTap` をタップ日の `dateField` 初期値付きで実行(新規・`FormPrefill` 経由)。
struct CalendarLeaf: View {
    @Environment(\.dslRuntime) private var rt
    let c: Component
    let ctx: RenderContext
    @State private var month: Date = Calendar.current.startOfMonth(for: .now)
    @State private var selected: Date = Calendar.current.startOfDay(for: .now)
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    private var cal: Calendar { Calendar.current }
    private var entityId: String { c.entity ?? "" }
    private var entity: Entity? { rt.dsl.entity(entityId) }
    private var dateField: String { c.dateField ?? "" }

    /// 日(startOfDay)→ その日のレコード
    private var byDay: [Date: [Record]] {
        var out: [Date: [Record]] = [:]
        for r in rt.records(for: entityId) {
            guard let d = FieldCoding.date(from: r[dateField]) else { continue }
            out[cal.startOfDay(for: d), default: []].append(r)
        }
        return out
    }

    var body: some View {
        let days = byDay
        VStack(alignment: .leading, spacing: 10) {
            header
            weekdayRow
            monthGrid(days)
            Divider()
            dayList(days[selected] ?? [])
        }
        .renderCard()
    }

    private var header: some View {
        HStack {
            if let title = c.title, !title.isEmpty {
                Text(title).font(.caption).foregroundStyle(.secondary)
            }
            Spacer()
            Button { shift(-1) } label: { Image(systemName: "chevron.left").frame(minWidth: 32, minHeight: 32) }
                .accessibilityLabel("前の月")
            Text(month.formatted(.dateTime.year().month(.wide)))
                .font(.headline.monospacedDigit())
                .lineLimit(1)
                .minimumScaleFactor(0.7)
                .frame(minWidth: 110)
                .accessibilityAddTraits(.isHeader)
            Button { shift(1) } label: { Image(systemName: "chevron.right").frame(minWidth: 32, minHeight: 32) }
                .accessibilityLabel("次の月")
        }
        .buttonStyle(.plain)
    }

    private var weekdayRow: some View {
        let symbols = cal.veryShortStandaloneWeekdaySymbols
        let first = cal.firstWeekday - 1
        return HStack(spacing: 0) {
            ForEach(0..<7, id: \.self) { i in
                Text(symbols[(first + i) % 7])
                    .font(.caption2).foregroundStyle(.secondary)
                    .frame(maxWidth: .infinity)
            }
        }
        .accessibilityHidden(true) // 曜日は各日のラベルに含めて読むので見出し行は飛ばす
    }

    private func monthGrid(_ days: [Date: [Record]]) -> some View {
        let cells = cellDates()
        return LazyVGrid(columns: Array(repeating: GridItem(.flexible(), spacing: 0), count: 7), spacing: 4) {
            ForEach(Array(cells.enumerated()), id: \.offset) { _, date in
                if let date {
                    dayCell(date, recordCount: days[date]?.count ?? 0)
                } else {
                    Color.clear.frame(height: 36)
                }
            }
        }
    }

    /// VoiceOver: 「10月24日 金曜日, 記録2件(, 今日)(, 選択中)」。日の数字は Dynamic Type で 36pt 枠に収まるよう縮小。
    private func dayCell(_ date: Date, recordCount: Int) -> some View {
        let isSelected = date == selected
        let isToday = cal.isDateInToday(date)
        let hasRecords = recordCount > 0
        return Button {
            selected = date
        } label: {
            VStack(spacing: 2) {
                Text("\(cal.component(.day, from: date))")
                    .font(.callout.monospacedDigit())
                    .fontWeight(isToday ? .bold : .regular)
                    .lineLimit(1)
                    .minimumScaleFactor(0.5)
                    .foregroundStyle(isSelected ? Color.white : (isToday ? Color.accentColor : Color.primary))
                Circle()
                    .fill(hasRecords ? (isSelected ? Color.white : Color.accentColor) : Color.clear)
                    .frame(width: 5, height: 5)
            }
            .frame(maxWidth: .infinity)
            .frame(height: 36)
            .background(isSelected ? Color.accentColor : Color.clear, in: RoundedRectangle(cornerRadius: 8))
        }
        .buttonStyle(.plain)
        .accessibilityLabel(date.formatted(.dateTime.month().day().weekday(.wide)) + (isToday ? "、今日" : ""))
        .accessibilityValue(hasRecords ? "記録\(recordCount)件" : "記録なし")
        .accessibilityAddTraits(isSelected ? .isSelected : [])
    }

    private func dayList(_ rows: [Record]) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                Text(selected.formatted(.dateTime.month().day().weekday()))
                    .font(.subheadline.weight(.semibold))
                Spacer()
                if c.onDateTap != nil {
                    Button {
                        addOnSelectedDay()
                    } label: {
                        Label("追加", systemImage: "plus").font(.subheadline)
                    }
                }
            }
            if rows.isEmpty {
                switch rt.emptyState(for: entityId) {
                case .loading: Text(EmptyPlaceholder.loadingText).font(.caption).foregroundStyle(.secondary)
                case .unavailable:
                    HStack(spacing: RenderTheme.Spacing.sm) {
                        Text(EmptyPlaceholder.unavailableText).font(.caption).foregroundStyle(.secondary)
                        Button(EmptyPlaceholder.retryLabel) { Task { await rt.load(entities: [entityId], force: true) } }
                            .font(.caption.weight(.semibold))
                    }
                case .empty: Text("予定はありません").font(.caption).foregroundStyle(.secondary)
                }
            } else {
                ForEach(rows) { row in
                    ListRow(item: ListItem(title: entity?.titleField ?? dateField), record: row, entity: entity)
                        .contentShape(Rectangle())
                        .onTapGesture {
                            guard let action = c.onDateTap else { return }
                            Task { await rt.run(action, ctx.actionContext(recordId: row.id)) }
                        }
                }
            }
        }
    }

    // MARK: - helpers

    private func shift(_ delta: Int) {
        if let m = cal.date(byAdding: .month, value: delta, to: month) {
            withAnimation(reduceMotion ? nil : .snappy) { month = cal.startOfMonth(for: m) }
        }
    }

    /// 選択日の dateField を初期値として積み、onDateTap(通常 navigate)を実行する。
    private func addOnSelectedDay() {
        guard let action = c.onDateTap else { return }
        if let f = entity?.field(dateField) {
            FormPrefill.set([dateField: .string(FieldCoding.encode(selected, for: f))])
        }
        Task { await rt.run(action, ctx.actionContext()) }
    }

    /// 先頭の空白(nil)+ 月の各日。
    private func cellDates() -> [Date?] {
        guard let range = cal.range(of: .day, in: .month, for: month) else { return [] }
        let weekday = cal.component(.weekday, from: month)
        let lead = (weekday - cal.firstWeekday + 7) % 7
        var out: [Date?] = Array(repeating: nil, count: lead)
        for day in range {
            out.append(cal.date(byAdding: .day, value: day - 1, to: month))
        }
        return out
    }
}

private extension Calendar {
    func startOfMonth(for date: Date) -> Date {
        self.date(from: dateComponents([.year, .month], from: date)) ?? date
    }
}
