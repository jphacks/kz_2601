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
                    .font(.system(size: 24, weight: .bold, design: .rounded))
                Text(L10n.tr("「\(rt.dsl.app.name)」の画面のつながり"))
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }
            Spacer(minLength: 0)
            Button { dismiss() } label: {
                Image(systemName: "xmark")
                    .font(.footnote.weight(.bold))
                    .foregroundStyle(.secondary)
                    .padding(Theme.Spacing.sm + 2)
                    .background(Theme.Colors.surface, in: Circle())
                    .overlay(Circle().strokeBorder(Theme.Colors.hairline))
            }
            .accessibilityLabel("閉じる")
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
                        .lineLimit(1)
                }
            }
        }
    }

    static func describe(_ e: ScreenGraph.Edge) -> String {
        let how: String = switch e.trigger {
        case .button(let label): L10n.tr("ボタン「\(label)」")
        case .toolbar(let label): L10n.tr("上部の「\(label)」")
        case .itemTap: L10n.tr("一覧の行")
        case .dateTap: L10n.tr("日付のタップ")
        case .afterSave: L10n.tr("保存したあと")
        case .other: L10n.tr("部品のタップ")
        }
        switch e.presentation {
        case .push: return how
        case .sheet: return how + L10n.tr("(シート)")
        case .fullScreen: return how + L10n.tr("(全画面)")
        }
    }
}

/// カードと矢印の図本体。カードは段ごとに中央揃えで並べ、矢印は Canvas でカードの後ろに描く。
private struct ScreenMapCanvas: View {
    let graph: ScreenGraph
    let hasTabs: Bool
    @Binding var selected: String?
    let availableWidth: CGFloat

    static let cardHeight: CGFloat = 66
    static let columnGap: CGFloat = 18
    static let rowGap: CGFloat = 64
    static let inset: CGFloat = Theme.Spacing.lg
    /// 到達できない段の上に置く見出しのぶん
    static let unreachableLabel: CGFloat = 26

    private var maxColumns: Int { max(1, graph.rows.map(\.count).max() ?? 1) }
    private var hasUnreachable: Bool { !graph.unreachable.isEmpty }

    /// 幅に収まるならカードを広げ、収まらなければ最小幅で横スクロール。
    private var cardWidth: CGFloat {
        let n = CGFloat(maxColumns)
        let fit = (availableWidth - Self.inset * 2 - Self.columnGap * (n - 1)) / n
        return min(150, max(104, fit))
    }

    private var contentWidth: CGFloat {
        max(availableWidth, Self.inset * 2 + CGFloat(maxColumns) * cardWidth + CGFloat(maxColumns - 1) * Self.columnGap)
    }

    private var contentHeight: CGFloat {
        let rows = CGFloat(graph.rows.count)
        return Self.inset * 2 + rows * Self.cardHeight + max(0, rows - 1) * Self.rowGap + (hasUnreachable ? Self.unreachableLabel : 0)
    }

    private func rowTop(_ row: Int) -> CGFloat {
        let extra = hasUnreachable && row == graph.rows.count - 1 ? Self.unreachableLabel : 0
        return Self.inset + CGFloat(row) * (Self.cardHeight + Self.rowGap) + extra
    }

    func frame(of node: ScreenGraph.Node) -> CGRect {
        let count = CGFloat(graph.rows[node.row].count)
        let rowWidth = count * cardWidth + (count - 1) * Self.columnGap
        let x = (contentWidth - rowWidth) / 2 + CGFloat(node.column) * (cardWidth + Self.columnGap)
        return CGRect(x: x, y: rowTop(node.row), width: cardWidth, height: Self.cardHeight)
    }

    var body: some View {
        ZStack(alignment: .topLeading) {
            arrows
            if hasUnreachable {
                Label(L10n.tr("どこからも行けない画面"), systemImage: "exclamationmark.triangle")
                    .font(.caption.weight(.semibold))
                    .foregroundStyle(Theme.Colors.warning)
                    .offset(x: Self.inset, y: rowTop(graph.rows.count - 1) - Self.unreachableLabel)
            }
            ForEach(graph.nodes) { node in
                let f = frame(of: node)
                ScreenCard(node: node, rootLabel: hasTabs ? L10n.tr("タブ") : L10n.tr("最初の画面"), isSelected: selected == node.id) {
                    selected = selected == node.id ? nil : node.id
                }
                .frame(width: f.width, height: f.height)
                .offset(x: f.minX, y: f.minY)
            }
        }
        .frame(width: contentWidth, height: contentHeight, alignment: .topLeading)
    }

    private var arrows: some View {
        Canvas { ctx, _ in
            let frames = Dictionary(uniqueKeysWithValues: graph.nodes.map { ($0.id, frame(of: $0)) })
            let pairs = Set(graph.links.map(\.id))
            for link in graph.links where link.from != link.to {
                guard let a = frames[link.from], let b = frames[link.to] else { continue }
                let from = graph.node(link.from), to = graph.node(link.to)
                // 上り・同じ段・往復の矢印は曲げて、逆向きの線やカードと重ならないようにする
                let curved = pairs.contains("\(link.to)->\(link.from)") || (to?.row ?? 0) <= (from?.row ?? 0)
                let path = ArrowGeometry.path(from: a, to: b, curved: curved)
                let active = selected == nil || selected == link.from || selected == link.to
                let color: Color = active ? Theme.Colors.brandA : .secondary
                let opacity = (from?.reachable ?? false) ? (active ? 0.9 : 0.25) : 0.25
                ctx.stroke(path.line, with: .color(color.opacity(opacity)),
                           style: StrokeStyle(lineWidth: active && selected != nil ? 2.4 : 1.6, lineCap: .round,
                                              dash: link.isModal ? [6, 4] : []))
                ctx.fill(path.head, with: .color(color.opacity(opacity)))
            }
        }
        .frame(width: contentWidth, height: contentHeight)
        .allowsHitTesting(false)
    }
}

/// 矢印の線と矢じり。線はカードの中心どうしを結び、カードの縁で切る。曲げる時は進行方向の左へ膨らませる(往復は自然に左右に分かれる)。
enum ArrowGeometry {
    static let bend: CGFloat = 34
    static let headLength: CGFloat = 9

    static func path(from a: CGRect, to b: CGRect, curved: Bool) -> (line: Path, head: Path) {
        let ca = CGPoint(x: a.midX, y: a.midY), cb = CGPoint(x: b.midX, y: b.midY)
        let dx = cb.x - ca.x, dy = cb.y - ca.y
        let len = max(1, hypot(dx, dy))
        let mid = CGPoint(x: (ca.x + cb.x) / 2, y: (ca.y + cb.y) / 2)
        let control = curved ? CGPoint(x: mid.x + dy / len * bend, y: mid.y - dx / len * bend) : mid
        let start = edgePoint(of: a, toward: control)
        let end = edgePoint(of: b, toward: control)
        var line = Path()
        line.move(to: start)
        if curved { line.addQuadCurve(to: end, control: control) } else { line.addLine(to: end) }

        let ux = end.x - control.x, uy = end.y - control.y
        let ul = max(1, hypot(ux, uy))
        let (nx, ny) = (ux / ul, uy / ul)
        let base = CGPoint(x: end.x - nx * headLength, y: end.y - ny * headLength)
        var head = Path()
        head.move(to: end)
        head.addLine(to: CGPoint(x: base.x - ny * headLength * 0.55, y: base.y + nx * headLength * 0.55))
        head.addLine(to: CGPoint(x: base.x + ny * headLength * 0.55, y: base.y - nx * headLength * 0.55))
        head.closeSubpath()
        return (line, head)
    }

    /// 矩形の中心から `p` へ向かう半直線と、矩形の縁(少し外側)との交点。
    static func edgePoint(of r: CGRect, toward p: CGPoint) -> CGPoint {
        let c = CGPoint(x: r.midX, y: r.midY)
        let dx = p.x - c.x, dy = p.y - c.y
        guard dx != 0 || dy != 0 else { return c }
        let hw = r.width / 2 + 3, hh = r.height / 2 + 3
        let t = min(dx == 0 ? .infinity : hw / abs(dx), dy == 0 ? .infinity : hh / abs(dy))
        return CGPoint(x: c.x + dx * t, y: c.y + dy * t)
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
