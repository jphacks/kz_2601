import SwiftUI
import MetamonDSL

/// `list`: entity のレコード一覧(filter/sort はクライアント側)。行タップで `onItemTap`、長押しで削除(swipeToDelete)。
/// 保存で増えた行(前回描画に無かった id)はブランドの薄い色で 1.2 秒ハイライト(Reduce Motion では省略)。
/// `item.toggle`(#108): 行頭の丸で boolean を反転して即保存(楽観的 UI。失敗したら戻す)。
struct ListLeaf: View {
    @Environment(\.dslRuntime) private var rt
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    let c: Component
    let ctx: RenderContext
    @State private var seenIds: Set<String>?
    @State private var highlightedId: String?
    /// 保存待ちのチェック状態(recordId → 新しい値)。`ListToggle` 参照
    @State private var toggleOverrides: [String: Bool] = [:]
    @State private var toggleTaps = 0

    var body: some View {
        let entityId = c.entity ?? ""
        let entity = rt.dsl.entity(entityId)
        let filtered = Aggregator.sorted(rt.filteredRecords(c), by: c.sort, entity: entity)
        let rows = searched(filtered, entity: entity)
        let ids = rows.map(\.id)
        let toggleField = ListToggle.field(c.item, in: entity)
        if rows.isEmpty {
            EmptyPlaceholder(icon: "tray", emptyText: c.emptyText ?? "まだありません", state: rt.emptyState(for: entityId),
                             cta: "最初の 1 件を追加しましょう") {
                Task { await rt.load(entities: [entityId], force: true) }
            }
            .onChange(of: ids, initial: true) { _, now in seenIds = Set(now) }
        } else {
            // 1 枚のカードに行を積み、行間は 1pt の hairline(iOS の inset grouped list と同じ読み味)
            LazyVStack(spacing: 0) {
                ForEach(Array(rows.enumerated()), id: \.element.id) { index, row in
                    if index > 0 { Hairline(inset: separatorInset) }
                    ListRow(item: c.item, record: row, entity: entity, showsChevron: c.onItemTap != nil,
                            toggleOn: toggleField.map { ListToggle.isOn(row, field: $0.id, overrides: toggleOverrides) },
                            onToggle: toggleField.map { f in { toggle(row, field: f.id, entity: entityId) } })
                        .background(highlightedId == row.id ? RenderTheme.Colors.brandSoft : Color.clear)
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
            .onChange(of: ids, initial: true) { _, now in markNewRows(now) }
            .sensoryFeedback(.selection, trigger: toggleTaps)
            #if DEBUG
            // ヘッドレス E2E 用: `METAMON_E2E_TOGGLE_TITLE=<title>` で、起動 3 秒後にその title の行の丸を 1 回タップしたのと同じ処理を行う
            .task {
                guard let target = ProcessInfo.processInfo.environment["METAMON_E2E_TOGGLE_TITLE"], let f = toggleField else { return }
                try? await Task.sleep(for: .seconds(3))
                let titleField = c.item?.title ?? entity?.displayTitleField ?? ""
                guard let row = rows.first(where: { rt.display($0[titleField], field: entity?.field(titleField)) == target }) else { return }
                print("[Metamon] E2E: toggling \(target)")
                toggle(row, field: f.id, entity: entityId)
            }
            #endif
        }
    }

    /// 楽観的に反転 → ローカル DB へ保存(同期キューが送る)→ 結果で保存待ち表示を外す(失敗ならストアの値 = 元に戻る)。
    private func toggle(_ row: Record, field: String, entity: String) {
        let next = !ListToggle.isOn(row, field: field, overrides: toggleOverrides)
        withAnimation(RenderTheme.Motion.select) { toggleOverrides[row.id] = next }
        toggleTaps += 1
        Task {
            let saved = await rt.updateRecord(entity: entity, recordId: row.id, data: [field: .bool(next)])
            if !saved { print("[Metamon] list toggle not saved: \(entity)/\(row.id)") }
            withAnimation(RenderTheme.Motion.select) { ListToggle.settle(&toggleOverrides, recordId: row.id, requested: next) }
        }
    }

    private func open(_ row: Record) {
        guard let action = c.onItemTap else { return }
        Task { await rt.run(action, ctx.actionContext(recordId: row.id)) }
    }

    /// 先頭のアイコン/画像の幅ぶんセパレータを内側に寄せる。
    private var separatorInset: CGFloat {
        let check: CGFloat = c.item?.toggle != nil ? 28 + RenderTheme.Spacing.row : 0
        if c.item?.image != nil { return RenderTheme.Spacing.md + check + 52 + RenderTheme.Spacing.row }
        if c.item?.icon != nil { return RenderTheme.Spacing.md + check + 36 + RenderTheme.Spacing.row }
        return RenderTheme.Spacing.md + check
    }

    /// 前回描画に無かった id をハイライト(初回描画・空 → 初回ロードは対象外)。
    private func markNewRows(_ now: [String]) {
        defer { seenIds = Set(now) }
        guard let seen = seenIds, !seen.isEmpty, !reduceMotion, let fresh = now.first(where: { !seen.contains($0) }) else { return }
        withAnimation(RenderTheme.Motion.select) { highlightedId = fresh }
        Task {
            try? await Task.sleep(for: .seconds(RenderTheme.Motion.highlightSeconds))
            withAnimation(.easeOut(duration: 0.4)) { if highlightedId == fresh { highlightedId = nil } }
        }
    }

    /// `searchField`(同画面の searchBar.field)の文字列で title/subtitle を部分一致(大小無視)フィルタ。
    private func searched(_ rows: [Record], entity: Entity?) -> [Record] {
        guard let key = c.searchField, let q = ctx.locals?.values[key]?.trimmingCharacters(in: .whitespaces), !q.isEmpty else { return rows }
        let fields = [c.item?.title ?? entity?.titleField, c.item?.subtitle].compactMap { $0 }
        return rows.filter { r in
            fields.contains { rt.display(r[$0], field: entity?.field($0)).localizedCaseInsensitiveContains(q) }
        }
    }
}
