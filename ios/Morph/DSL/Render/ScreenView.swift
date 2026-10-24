import SwiftUI

/// Route → Screen 定義を引いて描画する。NavigationStack の destination / sheet の中身。
struct ScreenView: View {
    @Environment(AppRuntime.self) private var rt
    let route: Route

    var body: some View {
        if let screen = rt.document.screens[route.screen] {
            let ctx = RenderContext(params: route.params)
            content(screen, ctx)
                .navigationTitle(rt.string(screen.title, ctx))
                .navigationBarTitleDisplayMode(.inline)
                .toolbar {
                    ToolbarItemGroup(placement: .topBarTrailing) {
                        ForEach(screen.toolbar) { DSLView(node: $0, ctx: ctx) }
                    }
                }
                .task(id: route) {
                    for action in screen.onAppear { await rt.run(action, ctx) }
                }
        } else {
            ContentUnavailableView("画面がありません: \(route.screen)", systemImage: "questionmark.square.dashed")
        }
    }

    @ViewBuilder
    private func content(_ screen: AppDocument.Screen, _ ctx: RenderContext) -> some View {
        if screen.scroll ?? true {
            ScrollView {
                DSLView(node: screen.body, ctx: ctx)
                    .padding(16)
            }
            .scrollDismissesKeyboard(.interactively)
            .background(Color(.systemGroupedBackground))
        } else {
            DSLView(node: screen.body, ctx: ctx)
        }
    }
}
