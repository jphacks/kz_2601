import SwiftUI
import MetamonDSL
import MetamonRenderer

/// 画面マップ(#116)。今のアプリの画面をカード、遷移を矢印で描く。
/// 段は `ScreenGraph`(根 = タブを上段、BFS の深さで下へ)。push は実線、sheet / fullScreen は破線、到達できない画面は薄く最後の段に。
/// カードを選ぶと出入りの一覧と「この画面について話す」が出る。
struct ScreenMapSheet: View {
    let onTalk: (ScreenContext) -> Void
    @Environment(AppRuntime.self) private var rt
    @Environment(\.dismiss) private var dismiss
    @State private var selected: String?
    @ScaledMetric(relativeTo: .title2) private var titleSize: CGFloat = 24

    private var graph: ScreenGraph { ScreenGraph(dsl: rt.dsl) }

    var body: some View {
        let graph = graph
        VStack(spacing: 0) {
            header
                .padding(.horizontal, Theme.Spacing.lg)
                .padding(.top, Theme.Spacing.lg)
            legend
                .padding(.horizontal, Theme.Spacing.lg)
                .padding(.top, Theme.Spacing.sm)
            GeometryReader { geo in
                ScrollView([.horizontal, .vertical], showsIndicators: false) {
                    ScreenMapCanvas(graph: graph, hasTabs: rt.dsl.app.hasTabs, selected: $selected, availableWidth: geo.size.width)
                        .frame(minWidth: geo.size.width, minHeight: geo.size.height, alignment: .top) // 小さい図は上に寄せる
                }
            }
            if let id = selected, let node = graph.node(id) {
                detail(node, graph: graph)
                    .transition(.move(edge: .bottom).combined(with: .opacity))
            } else {
                Text("画面をタップすると、その画面について話せます")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
                    .padding(.bottom, Theme.Spacing.lg)
            }
        }
        .background(Theme.Colors.canvas.ignoresSafeArea())
        .animation(.spring(duration: 0.35, bounce: 0.15), value: selected)
        .sensoryFeedback(.selection, trigger: selected)
        .presentationDetents([.large])
        .presentationDragIndicator(.visible)
        .onAppear {
            if let id = ScreenMapUI.shared.preselect, graph.node(id) != nil { selected = id }
        }
    }

    // MARK: - Sections

    private var header: some View {
        HStack(alignment: .top, spacing: Theme.Spacing.md) {
            VStack(alignment: .leading, spacing: Theme.Spacing.xs) {
                Text(ScreenMapCopy.title)
                    .font(.system(size: titleSize, weight: .bold, design: .rounded))
                Text(L10n.tr("「\(rt.dsl.app.name)」の画面のつながり"))
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }
            Spacer(minLength: 0)
            SheetCloseButton { dismiss() }
        }
    }

    private var legend: some View {
        HStack(spacing: Theme.Spacing.md) {
            legendItem(dashed: false, text: L10n.tr("画面を進む"))
            legendItem(dashed: true, text: L10n.tr("シートで開く"))
            Spacer(minLength: 0)
        }
        .font(.caption)
        .foregroundStyle(.secondary)
    }

    private func legendItem(dashed: Bool, text: String) -> some View {
        HStack(spacing: Theme.Spacing.xs) {
            Path { p in p.move(to: CGPoint(x: 0, y: 5)); p.addLine(to: CGPoint(x: 22, y: 5)) }
                .stroke(.secondary, style: StrokeStyle(lineWidth: 2, dash: dashed ? [4, 3] : []))
                .frame(width: 22, height: 10)
            Text(text)
        }
    }

    private func detail(_ node: ScreenGraph.Node, graph: ScreenGraph) -> some View {
        VStack(alignment: .leading, spacing: Theme.Spacing.sm) {
            HStack {
                Text(node.title).font(.headline)
                Text(node.id).font(.caption.monospaced()).foregroundStyle(.tertiary)
                Spacer()
                if !node.reachable {
                    Text("どこからも行けません")
                        .font(.caption.weight(.semibold))
                        .foregroundStyle(Theme.Colors.warning)
                }
            }
            linkList(L10n.tr("ここから行ける"), graph.outgoing(from: node.id), graph: graph, outgoing: true)
            linkList(L10n.tr("ここへ来られる"), graph.incoming(to: node.id), graph: graph, outgoing: false)
            Button {
                onTalk(ScreenContext(id: node.id, title: node.title))
            } label: {
                Label(L10n.tr("この画面について話す"), systemImage: "mic.fill")
                    .font(.subheadline.weight(.semibold))
                    .foregroundStyle(.white)
                    .frame(maxWidth: .infinity)
                    .frame(height: 48)
                    .background(Theme.brandGradient, in: Capsule())
            }
            .buttonStyle(.plain)
            .disabled(rt.isMorphing)
            .padding(.top, Theme.Spacing.xs)
            .accessibilityIdentifier("screenMap.talk")
        }
        .metamonCard()
        .padding(.horizontal, Theme.Spacing.md)
        .padding(.bottom, Theme.Spacing.sm)
    }

    @ViewBuilder
    private func linkList(_ label: String, _ links: [ScreenGraph.Link], graph: ScreenGraph, outgoing: Bool) -> some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(label).font(.caption.weight(.semibold)).foregroundStyle(.tertiary)
            if links.isEmpty {
                Text("なし").font(.footnote).foregroundStyle(.secondary)
            }
            ForEach(links) { link in
                let other = graph.node(outgoing ? link.to : link.from)?.title ?? ""
                HStack(alignment: .firstTextBaseline, spacing: Theme.Spacing.xs) {
                    Image(systemName: outgoing ? "arrow.right" : "arrow.left")
                        .font(.caption2.weight(.bold))
                        .foregroundStyle(Theme.brandGradient)
                    Text(other).font(.footnote.weight(.semibold))
                    Text(link.edges.map(Self.describe).joined(separator: L10n.tr("、")))
                        .font(.caption)
                        .foregroundStyle(.secondary)
                        .lineLimit(2)
                }
            }
        }
    }

    static func describe(_ e: ScreenGraph.Edge) -> String { ScreenMapCopy.describe(e) }
}

/// カードと矢印の図本体。並べ方と矢印の形は Mac と共有の `ScreenMapLayout`、矢印は Canvas でカードの後ろに描く。
private struct ScreenMapCanvas: View {
    let graph: ScreenGraph
    let hasTabs: Bool
    @Binding var selected: String?
    let availableWidth: CGFloat

    private var layout: ScreenMapLayout { ScreenMapLayout(graph: graph, availableWidth: availableWidth) }

    var body: some View {
        let layout = layout
        ZStack(alignment: .topLeading) {
            arrows(layout)
            if layout.hasUnreachable {
                Label(L10n.tr("どこからも行けない画面"), systemImage: "exclamationmark.triangle")
                    .font(.caption.weight(.semibold))
                    .foregroundStyle(Theme.Colors.warning)
                    .offset(x: layout.inset, y: layout.rowTop(graph.rows.count - 1) - layout.unreachableLabel)
            }
            ForEach(graph.nodes) { node in
                let f = layout.frame(of: node)
                ScreenCard(node: node, rootLabel: hasTabs ? L10n.tr("タブ") : L10n.tr("最初の画面"), isSelected: selected == node.id) {
                    selected = selected == node.id ? nil : node.id
                }
                .frame(width: f.width, height: f.height)
                .offset(x: f.minX, y: f.minY)
            }
        }
        .frame(width: layout.contentWidth, height: layout.contentHeight, alignment: .topLeading)
    }

    private func arrows(_ layout: ScreenMapLayout) -> some View {
        Canvas { ctx, _ in
            layout.drawArrows(in: ctx, selected: selected, accent: Theme.Colors.brandA)
        }
        .frame(width: layout.contentWidth, height: layout.contentHeight)
        .allowsHitTesting(false)
    }
}

/// 画面 1 枚分のカード。タブの根・フォームはバッジで区別、到達できない画面は薄く破線枠。
private struct ScreenCard: View {
    let node: ScreenGraph.Node
    let rootLabel: String
    let isSelected: Bool
    let action: () -> Void

    var body: some View {
        Button(action: action) {
            VStack(alignment: .leading, spacing: 4) {
                HStack(spacing: 4) {
                    Image(systemName: icon)
                        .font(.caption2.weight(.bold))
                        .foregroundStyle(node.reachable ? AnyShapeStyle(Theme.brandGradient) : AnyShapeStyle(.secondary))
                    Text(badge)
                        .font(.caption2.weight(.semibold))
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                }
                Text(node.title)
                    .font(.subheadline.weight(.semibold))
                    .foregroundStyle(.primary)
                    .lineLimit(2)
                    .minimumScaleFactor(0.8)
            }
            .padding(.horizontal, Theme.Spacing.sm + 2)
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
            .background(Theme.Colors.surface, in: RoundedRectangle(cornerRadius: Theme.Radius.sm + 4, style: .continuous))
            .overlay(
                RoundedRectangle(cornerRadius: Theme.Radius.sm + 4, style: .continuous)
                    .strokeBorder(isSelected ? AnyShapeStyle(Theme.brandGradient) : AnyShapeStyle(Theme.Colors.hairline),
                                  style: StrokeStyle(lineWidth: isSelected ? 2.5 : 1, dash: node.reachable ? [] : [5, 3]))
            )
            .shadow(color: Theme.Shadow.softColor, radius: isSelected ? 10 : 4, y: 3)
            .opacity(node.reachable ? 1 : 0.5)
        }
        .buttonStyle(CardPressStyle())
        .accessibilityLabel(node.title)
        .accessibilityHint(L10n.tr("タップで選択"))
        .accessibilityIdentifier("screenMap.node.\(node.id)")
    }

    private var icon: String {
        if node.isRoot { return "square.grid.2x2" }
        if node.isForm { return "square.and.pencil" }
        return "rectangle.portrait"
    }

    private var badge: String {
        if node.isRoot { return rootLabel }
        if node.isForm { return L10n.tr("フォーム") }
        return L10n.tr("画面")
    }
}

#Preview {
    let services = BuildConfig.make(environment: .mock)
    let rt = AppRuntime(document: (try? DSLExamples.document("navigation")) ?? .fallback, services: services)
    return ScreenMapSheet { _ in }
        .environment(rt)
}
