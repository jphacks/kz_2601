import SwiftUI

/// 描画中の文脈。画面 route と(フォーム画面なら)ドラフト。
struct RenderContext {
    var route: Route
    var form: FormDraft?

    func actionContext(recordId: String? = nil) -> ActionContext {
        ActionContext(route: route, form: form, recordId: recordId)
    }
}

/// Route → Screen 定義を引いて描画する。NavigationStack の destination / sheet の中身。
struct ScreenView: View {
    @Environment(AppRuntime.self) private var rt
    let route: Route
    @State private var form: FormDraft?

    var body: some View {
        if let screen = rt.dsl.screen(route.screen) {
            let ctx = RenderContext(route: route, form: form)
            content(screen, ctx)
                .navigationTitle(screen.title)
                .navigationBarTitleDisplayMode(screen.isVoiceScreen ? .inline : .large)
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
                    if route.stack != "main", (rt.paths[route.stack] ?? []).isEmpty {
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

    @ViewBuilder
    private func content(_ screen: Screen, _ ctx: RenderContext) -> some View {
        if screen.isVoiceScreen {
            VStack(spacing: 12) {
                ForEach(screen.components) { DSLView(component: $0, ctx: ctx) }
            }
            .padding(16)
        } else {
            ScrollView {
                VStack(alignment: .leading, spacing: 16) {
                    ForEach(screen.components) { DSLView(component: $0, ctx: ctx) }
                }
                .padding(16)
            }
            .scrollDismissesKeyboard(.interactively)
            .background(Color(.systemGroupedBackground))
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
