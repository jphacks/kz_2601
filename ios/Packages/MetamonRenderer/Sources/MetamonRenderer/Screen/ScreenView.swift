import SwiftUI
import MetamonDSL

/// Route → Screen 定義を引いて描画する。NavigationStack の destination / sheet の中身。
public struct ScreenView: View {
    @Environment(\.dslRuntime) private var rt
    @Environment(\.formToolbarAccessory) private var formToolbarAccessory
    let route: Route
    @State private var form: FormDraft?
    @State private var locals = ScreenLocals()

    public init(route: Route) { self.route = route }

    public var body: some View {
        if let screen = rt.dsl.screen(route.screen) {
            let ctx = RenderContext(route: route, form: form, locals: locals)
            GridScreenLayout(screen: screen, ctx: ctx)
                .background(RenderTheme.Colors.canvas)
                .modifier(FormSaveBar(screen: screen, ctx: ctx)) // saveRecord は画面下の固定バーへ(#89)
                .navigationTitle(screen.title)
                .navigationBarTitleDisplayMode(.inline)
                .toolbar {
                    // フォーム画面はホストの FAB を隠す代わりに、ナビバー右上(DSL のボタンより左)に小さな入口を置く(#83)
                    if screen.entity != nil, let accessory = formToolbarAccessory {
                        ToolbarItem(placement: .topBarTrailing) { accessory }
                    }
                    ToolbarItemGroup(placement: .topBarTrailing) {
                        ForEach(screen.toolbar.filter { tb in !FormSaveBar.pinned(in: screen).contains { $0.id == tb.id } }) { tb in
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
                        ToolbarItem(placement: .topBarLeading) {
                            Button(L("閉じる")) { rt.close(route) }
                        }
                    }
                }
                .task(id: route) {
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

    /// `screen.entity` があればドラフトを用意。recordId 付きなら既存レコードをロード(編集モード)。
    private func setUpForm(_ screen: Screen) {
        guard let entityId = screen.entity, let entity = rt.dsl.entity(entityId) else {
            form = nil
            return
        }
        let record = route.recordId.flatMap { id in rt.records(for: entityId).first { $0.id == id } }
        if form == nil || form?.entity.id != entityId || form?.recordId != record?.id {
            let draft = FormDraft(entity: entity, record: record)
            if record == nil {
                // calendar の日付タップなど、直前に積まれた初期値を新規フォームだけが受け取る
                for (k, v) in FormPrefill.consume(for: entity) { draft.values[k] = v }
            }
            form = draft
        }
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
