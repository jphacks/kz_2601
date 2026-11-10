import SwiftUI
import MetamonDSL

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
    /// ⌘C の対象の行(#1332)。クリックか Tab でフォーカスした行
    @FocusState private var focusedRowId: String?
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
                        .background(highlightedId == row.id ? RenderTheme.Colors.brandSoft : Color.clear)
                        .contentShape(Rectangle())
                        .onTapGesture { focusRow(row); open(row) }
                        #if os(macOS)
                        .focusable()
                        .focused($focusedRowId, equals: row.id)
                        #endif
                        // VoiceOver: onTapGesture は読まれないので、行をボタンとして宣言しダブルタップで同じ遷移を実行
                        .accessibilityAddTraits(c.onItemTap != nil ? .isButton : [])
                        .accessibilityHint(c.onItemTap != nil ? L("ダブルタップで開きます") : "")
                        .accessibilityAction { open(row) }
                        .contextMenu {
                            #if os(macOS)
                            Button(L("コピー"), systemImage: "doc.on.doc") { copy(row, entity: entity) }
                            if let photo = rowImage(row, entity: entity) {
                                ImageMenuItems(image: photo.image, title: photo.title, copyLabel: L("画像をコピー")) {
                                    imageToSave = .init(document: ImageFileDocument(image: photo.image),
                                                        fileName: ImageExport.fileName(title: photo.title, fallback: L("写真"), date: Date()))
                                }
                                Divider()
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
                            #endif
                            if deletable(row, entity: entityId) { Button(L("削除")) { delete(row, entity: entityId) } }
                        }
                        .recordDrag { ListRowCopyText.text(item: c.item, record: row, entity: entity, rt: rt) }
                }
            }
            // 行が少なくてもカードはセルの高さまで伸ばす(#84: 空白の下半分を作らない)。行は上詰め
            .frame(maxHeight: .infinity, alignment: .top)
            #if os(macOS)
            // ⌘C(#1332): フォーカスのある行を右クリックの「コピー」と同じテキストで
            .onCopyCommand { copyCommand(rows: rows, entity: entity) }
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
        guard let row = ListRowCopyCommand.target(focusedRowId: focusedRowId,
                                                  textInputActive: ListRowCopyCommand.textInputActive, rows: rows) else { return [] }
        let text = ListRowCopyText.text(item: c.item, record: row, entity: entity, rt: rt)
        return text.isEmpty ? [] : [NSItemProvider(object: text as NSString)]
    }

    /// 行の写真(item.image)。手元にある画像だけ(https・動画のサムネイルは除く)。名前は写真の項目名
    private func rowImage(_ row: Record, entity: Entity?) -> (image: PlatformImage, title: String)? {
        guard let field = c.item?.image, VideoValue(row[field]) == nil, let image = PhotoCodec.image(from: row[field]) else { return nil }
        return (image, entity?.field(field)?.name ?? L("写真"))
    }
    #endif

    /// クリックした行を ⌘C の対象にする(Mac)。
    private func focusRow(_ row: Record) {
        #if os(macOS)
        focusedRowId = row.id
        #endif
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
    /// 行を外(メモ・Finder・メール)へドラッグするとコピーと同じテキストを渡す(Mac のみ・#1334)。
    /// Mac のドラッグは少し動かしてから始まるので、行のクリックや中のボタンとはぶつからない。iOS は何もしない。
    @ViewBuilder
    func recordDrag(_ text: @escaping () -> String) -> some View {
        #if os(macOS)
        onDrag { ListRowCopyText.dragItem(text()) }
        #else
        self
        #endif
    }
}
