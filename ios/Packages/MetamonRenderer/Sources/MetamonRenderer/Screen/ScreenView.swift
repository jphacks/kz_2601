import SwiftUI
import MetamonDSL

/// Route → Screen 定義を引いて描画する。NavigationStack の destination / sheet の中身。
public struct ScreenView: View {
    @Environment(\.dslRuntime) private var rt
    @Environment(\.formToolbarAccessory) private var formToolbarAccessory
    let route: Route
    @State private var form: FormDraft?
    @State private var locals = ScreenLocals()
    /// 入力のあるフォームを閉じる前の確認(M10)
    @State private var confirmDiscard = false

    public init(route: Route) { self.route = route }

    public var body: some View {
        if let screen = rt.dsl.screen(route.screen) {
            let ctx = RenderContext(route: route, form: form, locals: locals)
            GridScreenLayout(screen: screen, ctx: ctx)
                .environment(\.dslNestingDepth, 0) // 部品から開いたシートの画面も 0 段から数える
                .background(RenderTheme.Colors.canvas)
                .modifier(FormSaveBar(screen: screen, ctx: ctx)) // saveRecord は画面下の固定バーへ(#89)
                .modifier(PublicFormNotice(screen: screen)) // #253 公開の entity のフォームに「誰でも見られます」
                .navigationTitle(screen.title)
                .navigationBarTitleDisplayMode(.inline)
                .toolbar {
                    // フォーム画面はホストの FAB を隠す代わりに、ナビバー右上(DSL のボタンより左)に小さな入口を置く(#83)
                    if screen.entity != nil, let accessory = formToolbarAccessory {
                        ToolbarItem(placement: .topBarTrailing) { accessory }
                    }
                    ToolbarItemGroup(placement: .topBarTrailing) {
                        // 同じフォームを開く button がある画面では右上の＋を出さない(#416)
                        ForEach(DuplicateAddControl.toolbar(of: screen, in: rt.dsl).filter { tb in !FormSaveBar.pinned(in: screen).contains { $0.id == tb.id } && ButtonLeaf.isVisible(action: tb.action, form: form) }) { tb in
                            Button {
                                Task { await rt.run(tb.action, ctx.actionContext()) }
                            } label: {
                                if let icon = tb.icon { Label(tb.label, systemImage: icon) } else { Text(tb.label) }
                            }
                        }
                    }
                    // 数値キーパッドには return が無いので、全入力共通でキーボード上に「完了」を出す(#143)
                    ToolbarItemGroup(placement: .keyboard) {
                        Spacer()
                        Button(L("完了")) { KeyboardState.dismiss() }.fontWeight(.semibold)
                    }
                    if route.stack == "sheet" || route.stack == "full", (rt.paths[route.stack] ?? []).isEmpty {
                        SheetCloseToolbarItem {
                            if hasUnsavedInput { confirmDiscard = true } else { rt.close(route) }
                        }
                    }
                    // M10: 積まれたフォームに入力があれば、標準の戻るの代わりに確認付きの戻るを出す
                    if guardsBack {
                        ToolbarItem(placement: .topBarLeading) {
                            Button { confirmDiscard = true } label: {
                                Image(systemName: "chevron.backward").fontWeight(.semibold)
                            }
                            .accessibilityLabel(L("戻る"))
                            .accessibilityIdentifier("form.back")
                        }
                    }
                }
                // 標準の戻るを隠すと、端から払って戻る操作も止まる(払って入力を失わない)
                .navigationBarBackButtonHidden(guardsBack)
                // M10: シートのフォームに入力があれば、下に払って閉じない(「閉じる」で破棄を確認する)
                .interactiveDismissDisabled(hasUnsavedInput)
                .confirmationDialog(L("編集内容を破棄しますか?"), isPresented: $confirmDiscard, titleVisibility: .visible) {
                    Button(L("破棄"), role: .destructive) { rt.close(route) }
                    Button(L("編集を続ける"), role: .cancel) {}
                } message: {
                    Text(L("保存していない入力は失われます。"))
                }
                .task(id: route) {
                    // 新規フォームは読み込みを待たずに用意する(FormPrefill は数秒で失効するため、通信待ちで落とさない)
                    if route.recordId == nil { setUpForm(screen) } // single(#267)は読み込み後に既存の 1 件へ差し替わる
                    await rt.load(entities: rt.dsl.entitiesNeeded(by: screen))
                    setUpForm(screen)
                }
                .refreshable {
                    await rt.load(entities: rt.dsl.entitiesNeeded(by: screen), force: true)
                }
        } else {
            ContentUnavailableView(L("画面がありません: \(route.screen)"), systemImage: "questionmark.square.dashed")
        }
    }

    /// 戻る・閉じるで入力を失うか(M10)。push された画面も、シート・全画面の中ならシートを払って閉じられる。
    private var hasUnsavedInput: Bool {
        Self.guardsDiscard(route: route, paths: rt.paths, form: form)
    }

    /// 積まれた画面で、戻る(ボタン・端から払う)を確認付きに替えるか(M10)。
    private var guardsBack: Bool {
        Self.isPushed(route, paths: rt.paths) && hasUnsavedInput
    }

    /// 閉じる・戻るの前に破棄を確認するか(M10): 保存していない入力があり、シート・全画面の中か push された画面。
    /// タブ・メインの根のフォームは離れる操作が無い(タブ切替では入力は残る)ので確認しない。
    static func guardsDiscard(route: Route, paths: [String: [Route]], form: FormDraft?) -> Bool {
        guard form?.hasUnsavedChanges == true else { return false }
        return route.stack == "sheet" || route.stack == "full" || isPushed(route, paths: paths)
    }

    /// `route` が NavigationStack に積まれた(根ではない)画面か。
    static func isPushed(_ route: Route, paths: [String: [Route]]) -> Bool {
        (paths[route.stack] ?? []).contains(route)
    }

    /// `screen.entity` があればドラフトを用意。recordId 付きなら既存レコードをロード(編集モード)。
    private func setUpForm(_ screen: Screen) {
        guard let entityId = screen.entity, let entity = rt.dsl.entity(entityId) else {
            form = nil
            return
        }
        let record = Self.formRecord(entity: entity, recordId: route.recordId, records: rt.records(for: entityId))
        if form == nil || form?.entity.id != entityId || form?.recordId != record?.id {
            form = Self.makeDraft(entity: entity, record: record, rt: rt)
        }
    }

    /// フォームが編集するレコード。single の entity(#267)は recordId によらず、その 1 件(無ければ nil = 初期値の新規)。
    static func formRecord(entity: Entity, recordId: String?, records: [Record]) -> Record? {
        if entity.single { return SingleRecord.pick(records) }
        return recordId.flatMap { id in records.first { $0.id == id } }
    }

    /// フォームのドラフト(既存レコードなら編集・権限、新規なら直前に積まれた初期値)。適合テストの描画の木(`RenderTree`)も同じ作り方を使う。
    @MainActor
    static func makeDraft(entity: Entity, record: Record?, rt: any DSLRuntime) -> FormDraft {
        let draft = FormDraft(entity: entity, record: record)
        if let record {
            draft.isReadOnly = !rt.canEdit(entity: entity.id, record: record)
            draft.canDelete = !entity.single && rt.canDelete(entity: entity.id, record: record) // single の 1 件は消さない(#267)
        } else {
            // calendar の日付タップなど、直前に積まれた初期値を新規フォームだけが受け取る
            for (k, v) in FormPrefill.consume(for: entity) { draft.values[k] = v }
            draft.markClean() // 直前に積まれた初期値は入力ではない(閉じても確認しない・M10)
        }
        return draft
    }
}

private struct FormToolbarAccessoryKey: EnvironmentKey {
    static let defaultValue: AnyView? = nil
}

public extension EnvironmentValues {
    /// フォーム画面(`screen.entity` あり・sheet/push どちらも)のナビバー右上にホストが差し込む小さな View(例: マイク)。
    /// 左下の FAB が全幅ボタンに被るのを避けるため、フォーム画面ではホストが FAB を隠してこちらを入口にする(#83)。
    var formToolbarAccessory: AnyView? {
        get { self[FormToolbarAccessoryKey.self] }
        set { self[FormToolbarAccessoryKey.self] = newValue }
    }
}

/// `root` を根にした NavigationStack(push 先も全部 ScreenView)。タブ・モーダル・メインで共通。
public struct ScreenStack: View {
    @Environment(\.dslRuntime) private var rt
    let root: Route

    public init(root: Route) { self.root = root }

    public var body: some View {
        NavigationStack(path: rt.pathBinding(root.stack)) {
            ScreenView(route: root)
                .navigationDestination(for: Route.self) { ScreenView(route: $0) }
        }
    }
}
