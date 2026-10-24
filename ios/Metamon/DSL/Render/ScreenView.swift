import SwiftUI
import MetamonDSL

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

/// 画面グリッド配置。
/// - 横 6 列(A-F)= 可視幅 ÷ 6、縦 1 行 = 可視高さ ÷ 12。`area` を持つ直下の子は矩形に絶対配置(セル内でスクロール)。
/// - `area` の無い子は「area で使われた最終行の次の行」から全幅で上から積む(フォールバック)。
/// - 13 行目以降 or フォールバックがはみ出せば画面全体が縦スクロール。voiceInput 画面(初期状態)はスクロールしない。
struct GridScreenLayout: View {
    let screen: Screen
    let ctx: RenderContext

    private let gap: CGFloat = 8
    private let inset: CGFloat = 12

    var body: some View {
        GeometryReader { geo in
            let cellW = (geo.size.width - inset * 2) / CGFloat(GridArea.columns)
            let cellH = (geo.size.height - inset * 2) / CGFloat(GridArea.visibleRows)
            let placed = screen.components.compactMap { c in c.gridArea.map { (c, $0) } }
            let stacked = screen.components.filter { $0.gridArea == nil }
            let lastRow = placed.map(\.1.row2).max() ?? 0
            let gridHeight = CGFloat(lastRow) * cellH

            let content = VStack(alignment: .leading, spacing: gap) {
                if !placed.isEmpty {
                    ZStack(alignment: .topLeading) {
                        Color.clear.frame(width: geo.size.width - inset * 2, height: gridHeight)
                        ForEach(placed, id: \.0.id) { c, a in
                            GridCell(component: c, ctx: ctx)
                                .frame(width: CGFloat(a.colSpan) * cellW - gap, height: CGFloat(a.rowSpan) * cellH - gap)
                                .offset(x: CGFloat(a.col1) * cellW + gap / 2, y: CGFloat(a.row1 - 1) * cellH + gap / 2)
                        }
                    }
                }
                ForEach(stacked) { DSLView(component: $0, ctx: ctx) }
            }
            .padding(inset)
            .frame(width: geo.size.width, alignment: .topLeading)

            if screen.isVoiceScreen {
                content
            } else {
                ScrollView {
                    content
                }
                .scrollDismissesKeyboard(.interactively)
                .scrollBounceBehavior(.basedOnSize)
            }
        }
    }
}

/// セル = 長方形の器。中身がはみ出す型(list / section / stack / text)は器の内側でスクロールする。
private struct GridCell: View {
    let component: Component
    let ctx: RenderContext

    private var scrollsInside: Bool {
        switch component.type {
        case .list, .section, .vstack, .hstack, .text: true
        default: false
        }
    }

    var body: some View {
        if scrollsInside {
            ScrollView(.vertical) {
                DSLView(component: component, ctx: ctx)
            }
            .scrollBounceBehavior(.basedOnSize)
            .clipShape(RoundedRectangle(cornerRadius: 12))
        } else {
            DSLView(component: component, ctx: ctx)
                .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
                .clipped()
        }
    }
}
