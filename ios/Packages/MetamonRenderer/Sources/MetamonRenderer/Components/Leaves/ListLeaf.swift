import SwiftUI
import MetamonDSL
#if os(macOS)
import AppKit
#endif

/// `list`: entity のレコード一覧(filter/sort はクライアント側)。行タップで `onItemTap`、長押しで削除(swipeToDelete)。
/// 行は `List` ではなく `LazyVStack` なので `.swipeActions` が効かない。縦スクロールとぶつかる自前のスワイプは作らず、
/// 長押しのメニューと VoiceOver の操作「削除」で消す(M11)。消した直後の「元に戻す」はホスト(`deleteRecord`)が出す。
/// 保存で増えた行(前回描画に無かった id)はブランドの薄い色で 1.2 秒ハイライト(Reduce Motion では省略)。
/// `item.toggle`(#108): 行頭の丸で boolean を反転して即保存(楽観的 UI。失敗したら戻す)。
/// 他人の共有レコード(#249): 丸は薄く描いて押すと理由を出し、長押しの削除は出さない(`RecordPermission`)。
/// `item.interest`(#507): 仲間探しのプロフィールの行に「つながりたい」(`InterestButton`・審査用の版には無い)。
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
    /// 押した直後の印の表示(recordId → 楽観的な状態・#255)。確定したら外す
    @State private var markOverrides: [String: MarkState] = [:]
    #if os(macOS)
    /// 選んでいる行(#1387)。クリックか ↑↓ で選び、⌘C・⌫ の対象にする(#1332)。⌘・⇧ クリックで複数(#1398)
    @State private var selection = ListRowMultiSelection()
    /// 複数の行を消す前の確認(消せる行の id)
    @State private var bulkDelete: [String]?
    /// 一覧にキーボードのフォーカスがあるか。Tab は行ごとでなく一覧に 1 回だけ止まる(#1387)
    @FocusState private var listFocused: Bool
    /// 行の写真の「画像を保存…」(#1333)
    @State private var imageToSave: ImageExport.Pending?
    #endif

    var body: some View {
        let entityId = c.entity ?? ""
        let entity = rt.dsl.entity(entityId)
        let rows = Self.rows(c, rt: rt, locals: ctx.locals)
        let ids = rows.map(\.id)
        let toggleField = ListToggle.field(c.item, in: entity)
        let mark = c.item?.mark.flatMap { id in entity?.isShared == true ? entity?.mark(id) : nil }
        let today = DSLClock.now()
        if rows.isEmpty {
            EmptyPlaceholder(icon: "tray", emptyText: c.emptyText ?? L("まだありません"), state: rt.emptyState(for: entityId),
                             cta: L("最初の 1 件を追加しましょう"),
                             retry: { Task { await rt.load(entities: [entityId], force: true) } },
                             onCTA: emptyAdd(entity: entityId))
            .onChange(of: ids, initial: true) { _, now in seenIds = Set(now) }
        } else {
            // 1 枚のカードに行を積み、行間は 1pt の hairline(iOS の inset grouped list と同じ読み味)
            LazyVStack(spacing: 0) {
                ForEach(Array(rows.enumerated()), id: \.element.id) { index, row in
                    if index > 0 { Hairline(inset: separatorInset) }
                    let editable = rt.canEdit(entity: entityId, record: row)
                    ListRow(item: c.item, record: row, entity: entity, showsChevron: c.onItemTap != nil,
                            toggleOn: toggleField.map { ListToggle.isOn(row, field: $0.id, daily: ListToggle.isDaily(c.item), overrides: toggleOverrides, today: today) },
                            onToggle: toggleField.map { f in { editable ? toggle(row, field: f.id, entity: entityId) : rt.notifyReadOnly(entity: entityId) } },
                            toggleEnabled: editable,
                            mark: mark, markState: mark.map { markOverrides[row.id] ?? row.markState($0.id) },
                            onMark: mark.map { m in { toggleMark(row, mark: m.id, entity: entityId) } },
                            accessory: interestAccessory(row, entity: entityId))
                        .background(rowBackground(row))
                        .contentShape(Rectangle())
                        .onTapGesture { tapRow(row, order: ids) }
                        .macHelp(c.onItemTap != nil ? L("クリックで開きます") : nil)
                        #if os(macOS)
                        .accessibilityAddTraits(selection.ids.contains(row.id) ? .isSelected : [])
                        #endif
                        // VoiceOver: onTapGesture は読まれないので、行をボタンとして宣言しダブルタップで同じ遷移を実行
                        .accessibilityAddTraits(c.onItemTap != nil ? .isButton : [])
                        .accessibilityHint(c.onItemTap != nil ? L("ダブルタップで開きます") : "")
                        .accessibilityAction { open(row) }
                        .contextMenu {
                            ComponentTalkMenuItem(component: c, ctx: ctx) // 行のメニューが先に出るので、ここにも置く(#1323)
                            #if os(macOS)
                            if selection.isMultiple && selection.ids.contains(row.id) {
                                // 選んだ行をまとめて(#1398)
                                Button(L("選んだ \(selection.ids.count) 件をコピー"), systemImage: "doc.on.doc") {
                                    copySelected(rows: rows, entity: entity)
                                }
                                if !bulkDeletable(rows: rows, entityId: entityId).isEmpty {
                                    Button(L("選んだ \(bulkDeletable(rows: rows, entityId: entityId).count) 件を削除"),
                                           systemImage: "trash", role: .destructive) {
                                        bulkDelete = bulkDeletable(rows: rows, entityId: entityId)
                                    }
                                }
                                Divider()
                            }
                            Button(L("コピー"), systemImage: "doc.on.doc") { copy(row, entity: entity) }
                            if rows.count > 1 {
                                // 一覧全体(#1035): 書き出しと同じ列の CSV。⌘A → ⌘C と同じ中身
                                Button(L("すべての行をコピー(CSV)"), systemImage: "tablecells") {
                                    copyCSV(rows, entity: entity)
                                }
                            }
                            if let photo = rowImage(row, entity: entity) {
                                ImageMenuItems(image: photo.image, title: photo.title, copyLabel: L("画像をコピー")) {
                                    imageToSave = .init(document: ImageFileDocument(image: photo.image),
                                                        fileName: ImageExport.fileName(title: photo.title, fallback: L("写真"), date: Date()))
                                }
                                Divider()
                            }
                            #endif
                            #if os(iOS)
                            // 記録を項目つきで別の生成アプリへ(#1871)
                            if let entity, RecordPasteboard.payload(record: row, entity: entity, rt: rt) != nil {
                                Button(L("記録をコピー"), systemImage: "doc.on.doc") { RecordPasteboard.copy(record: row, entity: entity, rt: rt) }
                            }
                            #endif
                            if deletable(row, entity: entityId) {
                                Button(L("削除"), systemImage: "trash", role: .destructive) { delete(row, entity: entityId) }
                            }
                            ModerationMenuItems(entity: entityId, record: row)
                        }
                        .accessibilityActions {
                            #if os(macOS)
                            Button(L("コピー")) { copy(row, entity: entity) }
                            // 行の写真も右クリックと同じく VoiceOver からコピーできる(#1370)
                            if let photo = rowImage(row, entity: entity) {
                                Button(L("画像をコピー")) { PlatformPasteboard.setImage(photo.image) }
                            }
                            #endif
                            #if os(iOS)
                            if let entity, RecordPasteboard.payload(record: row, entity: entity, rt: rt) != nil {
                                Button(L("記録をコピー")) { RecordPasteboard.copy(record: row, entity: entity, rt: rt) }
                            }
                            #endif
                            if deletable(row, entity: entityId) { Button(L("削除")) { delete(row, entity: entityId) } }
                            ModerationAccessibilityActions(rt: rt, entity: entityId, record: row)
                        }
                        .recordDrag { rowDragItem(row, rows: rows, entity: entity) }
                }
            }
            // 行が少なくてもカードはセルの高さまで伸ばす(#84: 空白の下半分を作らない)。行は上詰め
            .frame(maxHeight: .infinity, alignment: .top)
            #if os(macOS)
            // 一覧ごとに 1 つのフォーカス(#1387)。青いリングは出さず、選んでいる行を自前の色で見せる。↑↓ で行を移り、Return で開く
            .focusable()
            .focusEffectDisabled()
            .focused($listFocused)
            .onMoveCommand { moveSelection($0, ids: ids) }
            .onKeyPress(.return) { openSelected(rows) }
            .onChange(of: listFocused) { _, focused in
                if focused && selection.ids.isEmpty { selection.select(ListRowSelection.onFocus(current: selection.cursor, ids: ids)) }
            }
            // 一覧が空になって出し直した時も(行の束は作り直しで、変わった時の知らせが来ない)、もう無い行を選択から外す
            .onChange(of: ids, initial: true) { _, now in selection.prune(order: now) }
            .confirmationDialog(L("選んだ \(bulkDelete?.count ?? 0) 件を削除しますか?"),
                                isPresented: Binding(get: { bulkDelete != nil }, set: { if !$0 { bulkDelete = nil } }),
                                titleVisibility: .visible) {
                Button(L("削除"), role: .destructive) { deleteSelected(entityId: entityId) }
            }
            // ⌘C(#1332): フォーカスのある行を右クリックの「コピー」と同じテキストで
            .onCopyCommand { copyCommand(rows: rows, entity: entity) }
            // ⌘A(#1035): 行を全部選ぶ。続けて ⌘C で一覧全体を CSV に(入力欄が文字を受けている時は入力欄の全選択に任せる)
            .onCommand(#selector(NSResponder.selectAll(_:))) {
                guard listFocused, !ListRowCopyCommand.textInputActive else { return }
                selection.selectAll(order: ids)
            }
            // ⌫: フォーカスのある行を、確認を挟んでから消す(消せない行では何もしない)
            .onDeleteCommand { deleteCommand(rows: rows, entity: entity, entityId: entityId) }
            #endif
            .renderCard(padding: 0)
            #if os(macOS)
            .imageSaveExporter($imageToSave)
            #endif
            .onChange(of: ids, initial: true) { _, now in markNewRows(now) }
            .sensoryFeedback(.selection, trigger: toggleTaps)
            .task(id: c.item?.interest == true ? entityId : "") { await loadInterestStates() }
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
            // `METAMON_E2E_MARK_TITLE=<title>`: 起動 3 秒後にその title の行の印を 1 回押す(#255 のスクリーンショット用)
            .task {
                guard let target = ProcessInfo.processInfo.environment["METAMON_E2E_MARK_TITLE"], let m = mark else { return }
                try? await Task.sleep(for: .seconds(3))
                let titleField = c.item?.title ?? entity?.displayTitleField ?? ""
                guard let row = rows.first(where: { rt.display($0[titleField], field: entity?.field(titleField)) == target }) else { return }
                print("[Metamon] E2E: marking \(target)")
                toggleMark(row, mark: m.id, entity: entityId)
            }
            #endif
        }
    }

    /// 楽観的に反転 → ローカル DB へ保存(同期キューが送る)→ 結果で保存待ち表示を外す(失敗ならストアの値 = 元に戻る)。
    private func toggle(_ row: Record, field: String, entity: String) {
        let daily = ListToggle.isDaily(c.item), today = DSLClock.now()
        let next = !ListToggle.isOn(row, field: field, daily: daily, overrides: toggleOverrides, today: today)
        withAnimation(RenderTheme.Motion.select) { toggleOverrides[row.id] = next }
        toggleTaps += 1
        Task {
            _ = await OptimisticSave.update(rt, entity: entity, recordId: row.id, data: ListToggle.patch(row, field: field, on: next, daily: daily, today: today))
            withAnimation(RenderTheme.Motion.select) { ListToggle.settle(&toggleOverrides, recordId: row.id, requested: next) }
        }
    }

    /// 印(#255): 押した瞬間に塗りと件数を変え、ホストが確定した状態(失敗なら元の状態)に戻す。
    private func toggleMark(_ row: Record, mark: String, entity: String) {
        let next = (markOverrides[row.id] ?? row.markState(mark)).toggled()
        withAnimation(RenderTheme.Motion.select) { markOverrides[row.id] = next }
        toggleTaps += 1
        Task {
            let settled = await rt.marks?.toggleMark(entity: entity, recordId: row.id, mark: mark)
            if settled == nil { print("[Metamon] mark not saved: \(entity)/\(row.id) \(mark)") }
            withAnimation(RenderTheme.Motion.select) { if markOverrides[row.id] == next { markOverrides[row.id] = nil } }
        }
    }

    /// `item.interest`(#507): 他の人のプロフィールの行に「つながりたい」。ホストが仲間探しを持たない版では nil。
    private func interestAccessory(_ row: Record, entity: String) -> RowAccessory? {
        #if APP_STORE_REVIEW
        return nil
        #else
        guard c.item?.interest == true, let host = rt.matching, let state = host.interestState(entity: entity, record: row) else { return nil }
        return InterestButton.accessory(state: state, busy: host.interestBusy(record: row)) {
            toggleTaps += 1
            Task { await host.sendInterest(entity: entity, record: row) }
        }
        #endif
    }

    /// 行のボタンの出し分けに、届いた / 送った / つながったを読む(#507)。
    private func loadInterestStates() async {
        #if !APP_STORE_REVIEW
        guard c.item?.interest == true else { return }
        await rt.matching?.refreshMatches(Set(MatchBox.allCases))
        #endif
    }

    /// 削除を出すか: `swipeToDelete` の一覧で、消せる記録(他人の共有の記録は消せない・#249)だけ。
    private func deletable(_ row: Record, entity: String) -> Bool {
        c.swipeToDelete == true && rt.canDelete(entity: entity, record: row)
    }

    private func delete(_ row: Record, entity: String) {
        Task { await rt.run(Action(type: .deleteRecord, entity: entity), ctx.actionContext(recordId: row.id)) }
    }

    /// 右クリックの「コピー」(Mac・#1332): 行に見えている項目をテキストにしてクリップボードへ。
    private func copy(_ row: Record, entity: Entity?) {
        let text = ListRowCopyText.text(item: c.item, record: row, entity: entity, rt: rt)
        guard !text.isEmpty else { return }
        PlatformPasteboard.setString(text)
    }

    #if os(macOS)
    private func copyCommand(rows: [Record], entity: Entity?) -> [NSItemProvider] {
        if selection.isMultiple {
            guard listFocused, !ListRowCopyCommand.textInputActive else { return [] }
            let csv = ListRowCopyText.csv(records: selectedRecords(rows), entity: entity, rt: rt)
            return csv.isEmpty ? [] : [NSItemProvider(object: csv as NSString)]
        }
        guard let row = ListRowCopyCommand.target(focusedRowId: keyboardRowId,
                                                  textInputActive: ListRowCopyCommand.textInputActive, rows: rows) else { return [] }
        let text = ListRowCopyText.text(item: c.item, record: row, entity: entity, rt: rt)
        return text.isEmpty ? [] : [NSItemProvider(object: text as NSString)]
    }

    private func deleteCommand(rows: [Record], entity: Entity?, entityId: String) {
        if selection.isMultiple {
            guard listFocused, !ListRowCopyCommand.textInputActive else { return }
            let ids = bulkDeletable(rows: rows, entityId: entityId)
            if !ids.isEmpty { bulkDelete = ids }
            return
        }
        guard let row = ListRowDeleteCommand.target(focusedRowId: keyboardRowId, textInputActive: ListRowCopyCommand.textInputActive,
                                                    rows: rows, deletable: { deletable($0, entity: entityId) }) else { return }
        let titleField = c.item?.title ?? entity?.displayTitleField ?? ""
        var action = Action(type: .deleteRecord, entity: entityId)
        action.confirmMessage = ListRowDeleteCommand.confirmMessage(title: rt.display(row[titleField], field: entity?.field(titleField)),
                                                                    fallback: entity?.name ?? "")
        Task { await rt.run(action, ctx.actionContext(recordId: row.id)) }
    }

    /// ⌘C・⌫ の対象: 一覧にフォーカスがある時の選んでいる行(⌘ クリックで外した行は対象にしない)
    private var keyboardRowId: String? { listFocused ? selection.single : nil }

    /// 選んでいる行(一覧に見えている順)
    private func selectedRecords(_ rows: [Record]) -> [Record] { rows.filter { selection.ids.contains($0.id) } }

    /// 選んでいる行のうち消せるもの(他人の共有の記録などは除く)
    private func bulkDeletable(rows: [Record], entityId: String) -> [String] {
        selectedRecords(rows).filter { deletable($0, entity: entityId) }.map(\.id)
    }

    /// 選んだ行をまとめてコピー: 書き出しと同じ列の CSV
    private func copySelected(rows: [Record], entity: Entity?) {
        copyCSV(selectedRecords(rows), entity: entity)
    }

    private func copyCSV(_ records: [Record], entity: Entity?) {
        let csv = ListRowCopyText.csv(records: records, entity: entity, rt: rt)
        guard !csv.isEmpty else { return }
        PlatformPasteboard.setString(csv)
    }

    /// 確認の後に 1 件ずつ消す。「元に戻す」で戻るのは最後の 1 件(ホストの取り消しは 1 件ずつ)
    private func deleteSelected(entityId: String) {
        guard let ids = bulkDelete else { return }
        bulkDelete = nil
        selection.select(nil)
        Task {
            for id in ids { await rt.run(Action(type: .deleteRecord, entity: entityId), ctx.actionContext(recordId: id)) }
        }
    }

    private func moveSelection(_ direction: MoveCommandDirection, ids: [String]) {
        let move: ListRowSelection.Move
        switch direction {
        case .up: move = .up
        case .down: move = .down
        default: return
        }
        if NSEvent.modifierFlags.contains(.shift) {
            selection.extend(move, order: ids) // ⇧↑↓ で範囲を広げる(#1398)
        } else {
            selection.select(ListRowSelection.moved(from: selection.cursor, move, ids: ids))
        }
    }

    /// Return: 選んでいる行を開く(行のクリックと同じ)。入力欄が文字を受けている時は任せる
    private func openSelected(_ rows: [Record]) -> KeyPress.Result {
        guard c.onItemTap != nil, !selection.isMultiple, !ListRowCopyCommand.textInputActive,
              let row = rows.first(where: { $0.id == selection.single }) else { return .ignored }
        open(row)
        return .handled
    }

    /// 行の写真(item.image)。手元にある画像だけ(https・動画のサムネイルは除く)。名前は写真の項目名
    private func rowImage(_ row: Record, entity: Entity?) -> (image: PlatformImage, title: String)? {
        guard let field = c.item?.image, VideoValue(row[field]) == nil, let image = PhotoCodec.image(from: row[field]) else { return nil }
        return (image, entity?.field(field)?.name ?? L("写真"))
    }
    #endif

    /// 行のクリック。Mac は行を選んで一覧にフォーカスを移す(⌘C・⌫・↑↓ がこの行から効く)。
    /// ⌘ クリックは選択に足し引き、⇧ クリックは範囲を選ぶだけで、行は開かない(#1398)
    private func tapRow(_ row: Record, order: [String]) {
        #if os(macOS)
        listFocused = true
        let flags = NSEvent.modifierFlags
        if flags.contains(.command) { return selection.click(row.id, .toggle, order: order) }
        if flags.contains(.shift) { return selection.click(row.id, .extend, order: order) }
        selection.select(row.id)
        #endif
        open(row)
    }

    /// 行を外へドラッグする時に渡すもの。選んだ複数の行の 1 つならまとめて CSV、それ以外はその行のテキスト(#1334)
    private func rowDragItem(_ row: Record, rows: [Record], entity: Entity?) -> NSItemProvider {
        #if os(macOS)
        if selection.isMultiple && selection.ids.contains(row.id) {
            let csv = ListRowCopyText.csv(records: selectedRecords(rows), entity: entity, rt: rt)
            return ListRowCopyText.csvDragItem(csv, title: entity?.name ?? "")
        }
        #endif
        return ListRowCopyText.dragItem(ListRowCopyText.text(item: c.item, record: row, entity: entity, rt: rt))
    }

    /// 行の背景: 保存で増えた行の一瞬のハイライト、Mac は選んでいる行(フォーカスが無い時は薄いグレー)
    private func rowBackground(_ row: Record) -> Color {
        if highlightedId == row.id { return RenderTheme.Colors.brandSoft }
        #if os(macOS)
        if selection.ids.contains(row.id) { return listFocused ? RenderTheme.Colors.brandSoftStroke : Color.primary.opacity(0.06) }
        #endif
        return .clear
    }

    private func open(_ row: Record) {
        guard let action = c.onItemTap else { return }
        Task { await rt.run(action, ctx.actionContext(recordId: row.id)) }
    }

    /// 空の一覧の案内を押した時: 同じ画面の追加ボタンと同じフォームを開く(M12)。無ければ nil(案内は文字だけ)。
    private func emptyAdd(entity: String) -> (() -> Void)? {
        guard let screen = rt.dsl.screen(ctx.route.screen),
              let action = DuplicateAddControl.emptyListAdd(entity: entity, on: screen, in: rt.dsl) else { return nil }
        return { Task { await rt.run(action, ctx.actionContext()) } }
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

    /// 表示する行(filter → sort → 検索)。適合テストの描画の木(`RenderTree`)も同じ行を使う。
    @MainActor
    static func rows(_ c: Component, rt: any DSLRuntime, locals: ScreenLocals?) -> [Record] {
        let entity = rt.dsl.entity(c.entity ?? "")
        let filtered = Aggregator.sorted(rt.filteredRecords(c), by: c.sort, entity: entity)
        return searched(c, filtered, entity: entity, rt: rt, locals: locals)
    }

    /// `searchField`(同画面の searchBar.field)の文字列で title/subtitle を部分一致(大小無視)フィルタ。
    @MainActor
    private static func searched(_ c: Component, _ rows: [Record], entity: Entity?, rt: any DSLRuntime, locals: ScreenLocals?) -> [Record] {
        guard let key = c.searchField, let q = locals?.values[key]?.trimmingCharacters(in: .whitespaces), !q.isEmpty else { return rows }
        let fields = [c.item?.title ?? entity?.titleField, c.item?.subtitle].compactMap { $0 }
        return rows.filter { r in
            fields.contains { rt.display(r[$0], field: entity?.field($0)).localizedCaseInsensitiveContains(q) }
        }
    }
}

private extension View {
    /// 行を外(メモ・Finder・メール)へドラッグすると、コピーと同じテキスト(複数を選んでいれば CSV)を渡す(Mac のみ・#1334・#1398)。
    /// Mac のドラッグは少し動かしてから始まるので、行のクリックや中のボタンとはぶつからない。iOS は何もしない。
    @ViewBuilder
    func recordDrag(_ item: @escaping () -> NSItemProvider) -> some View {
        #if os(macOS)
        onDrag(item)
        #else
        self
        #endif
    }
}
