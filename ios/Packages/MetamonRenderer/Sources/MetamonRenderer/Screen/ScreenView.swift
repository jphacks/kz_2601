import SwiftUI
import MetamonDSL

/// Route → Screen 定義を引いて描画する。NavigationStack の destination / sheet の中身。
public struct ScreenView: View {
    @Environment(\.dslRuntime) private var rt
    let route: Route
    @State private var form: FormDraft?

    public init(route: Route) { self.route = route }

    public var body: some View {
        if let screen = rt.dsl.screen(route.screen) {
            let ctx = RenderContext(route: route, form: form)
            GridScreenLayout(screen: screen, ctx: ctx)
                .background(Color(.systemGroupedBackground))
                .navigationTitle(screen.title)
                .navigationBarTitleDisplayMode(.inline)
                .toolbar {
                    ToolbarItemGroup(placement: .topBarTrailing) {
                        ForEach(screen.toolbar) { tb in
                            Button {
                                Task { await rt.run(tb.action, ctx.actionContext()) }
                            } label: {
                                if let icon = tb.icon { Label(tb.label, systemImage: icon) } else { Text(tb.label) }
                            }
                        }
                    }
                    if route.stack == "sheet" || route.stack == "full", (rt.paths[route.stack] ?? []).isEmpty {
                        ToolbarItem(placement: .topBarLeading) {
                            Button("閉じる") { rt.close(route) }
                        }
                    }
                }
                .task(id: route) {
                    await rt.load(entities: screen.referencedEntities)
                    setUpForm(screen)
                }
                .refreshable {
                    await rt.load(entities: screen.referencedEntities, force: true)
                }
        } else {
            ContentUnavailableView("画面がありません: \(route.screen)", systemImage: "questionmark.square.dashed")
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
            form = FormDraft(entity: entity, record: record)
        }
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
