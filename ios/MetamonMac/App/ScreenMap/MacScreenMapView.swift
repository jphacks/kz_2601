import SwiftUI
import MetamonDSL
import MetamonRenderer

/// 画面マップの Mac 版(#970・iOS の `ScreenMapSheet`・#116)。中央のアプリの上にシートで出す。
/// 図(段・カードの置き場所・矢印)は iOS と共有の `ScreenGraph` / `ScreenMapLayout`。
/// - クリックで選ぶ → 右に出入りの一覧と「この画面を開く」「この画面について話す」。ダブルクリックでその画面を開く
/// - 一覧の行から開く画面(詳細・フォーム)は「記録を選んで開く」でその記録の中身を出す(#1322)。記録が無ければ空で開くと知らせる
/// - キーボード: 矢印キーでカードを選ぶ・Return で開く・⌘+ / ⌘- / ⌘0 で拡大 / 縮小 / 等倍・Esc で閉じる
/// - トラックパッドのピンチでも拡大・縮小。大きい図は縦横にスクロール
/// - VoiceOver: カードは上の段から順に読む。既定の操作で開く・「この画面について話す」は操作の一覧から
/// シートにしたのは、図が中央のアプリ(とそのランタイム)1 つに属し、開く・話すとすぐ閉じて中央に戻るため(別ウインドウだとどのアプリの図か分かりにくい)。
struct MacScreenMapView: View {
    let dsl: AppDSL
    /// 開いた時に選んでおく画面(中央で今見ている画面)
    let current: String?
    let onOpen: (String) -> Void
    let onTalk: (ScreenContext) -> Void
    let onClose: () -> Void
    /// 記録の種類(entity)ごとの、選べる記録(#1322)
    var records: (String) -> [ScreenMapRecordChoice] = { _ in [] }
    /// 画面をその記録で開く(#1322)
    var onOpenRecord: (String, String) -> Void = { _, _ in }

    @State private var selected: String?
    @State private var zoom: CGFloat = 1
    @State private var pinchBase: CGFloat = 1
    @FocusState private var mapFocused: Bool
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    static let size = CGSize(width: 880, height: 620)
    static let zoomRange: ClosedRange<CGFloat> = 0.5...2
    static let zoomStep: CGFloat = 0.25

    private var graph: ScreenGraph { ScreenGraph(dsl: dsl) }

    /// 一覧の行(onItemTap)で開く画面なら、その一覧の記録の種類。それ以外の画面は nil(記録を選ばずに開く)
    static func recordEntity(opening screen: String, in dsl: AppDSL) -> String? {
        for s in dsl.screens {
            for c in s.allComponents where c.onItemTap?.screen == screen {
                if let entity = c.entity, dsl.entity(entity) != nil { return entity }
            }
        }
        return nil
    }

    var body: some View {
        let graph = graph
        VStack(spacing: 0) {
            header
            Divider()
            if dsl.isVoiceOnly || graph.nodes.isEmpty {
                MacEmptyState(pose: .point, title: "このアプリには画面がありません",
                              message: "声だけで使うアプリは、画面のつながりがありません。")
            } else {
                HStack(spacing: 0) {
                    map(graph)
                    Divider()
                    MacScreenMapDetail(graph: graph, selected: selected, hasTabs: dsl.app.hasTabs,
                                       recordChoices: selected.flatMap { Self.recordEntity(opening: $0, in: dsl) }.map(records),
                                       open: onOpen, openRecord: onOpenRecord, talk: talk,
                                       select: { select($0, announce: true) })
                        .frame(width: 280)
                }
            }
        }
        .frame(minWidth: 640, idealWidth: Self.size.width, minHeight: 480, idealHeight: Self.size.height)
        .background(.background)
        .onAppear {
            selected = current.flatMap { graph.node($0) != nil ? $0 : nil }
            mapFocused = true
        }
    }

    // MARK: - 見出し

    private var header: some View {
        HStack(spacing: MacTheme.Spacing.group) {
            VStack(alignment: .leading, spacing: 2) {
                Text(ScreenMapCopy.title).font(.headline)
                Text(L10n.tr("「\(dsl.app.name)」の画面のつながり"))
                    .font(.subheadline).foregroundStyle(.secondary).lineLimit(1)
            }
            Spacer()
            legend
            zoomControls
            Button(action: onClose) { Image(systemName: "xmark.circle.fill").font(.title3) }
                .buttonStyle(.plain)
                .foregroundStyle(.secondary)
                .keyboardShortcut(.cancelAction)
                .help("閉じる")
                .accessibilityLabel("閉じる")
        }
        .padding(.horizontal, MacTheme.Spacing.window)
        .padding(.vertical, MacTheme.Spacing.group)
    }

    private var legend: some View {
        HStack(spacing: MacTheme.Spacing.group) {
            legendItem(dashed: false, text: L10n.tr("画面を進む"))
            legendItem(dashed: true, text: L10n.tr("シートで開く"))
        }
        .font(.caption)
        .foregroundStyle(.secondary)
        .accessibilityElement(children: .combine)
    }

    private func legendItem(dashed: Bool, text: String) -> some View {
        HStack(spacing: 4) {
            Path { p in p.move(to: CGPoint(x: 0, y: 5)); p.addLine(to: CGPoint(x: 22, y: 5)) }
                .stroke(.secondary, style: StrokeStyle(lineWidth: 2, dash: dashed ? [4, 3] : []))
                .frame(width: 22, height: 10)
                .accessibilityHidden(true)
            Text(text)
        }
    }

    private var zoomControls: some View {
        ControlGroup {
            Button { setZoom(zoom - Self.zoomStep) } label: { Image(systemName: "minus.magnifyingglass") }
                .keyboardShortcut("-", modifiers: .command)
                .disabled(zoom <= Self.zoomRange.lowerBound)
                .help("縮小")
                .accessibilityLabel("縮小")
            Button { setZoom(1) } label: { Text(zoom, format: .percent.precision(.fractionLength(0))).monospacedDigit() }
                .keyboardShortcut("0", modifiers: .command)
                .help("実際のサイズ")
                .accessibilityLabel("実際のサイズ")
            Button { setZoom(zoom + Self.zoomStep) } label: { Image(systemName: "plus.magnifyingglass") }
                .keyboardShortcut("+", modifiers: .command)
                .disabled(zoom >= Self.zoomRange.upperBound)
                .help("拡大")
                .accessibilityLabel("拡大")
        }
        .controlGroupStyle(.navigation)
        .fixedSize()
    }

    private func setZoom(_ value: CGFloat) {
        let next = min(Self.zoomRange.upperBound, max(Self.zoomRange.lowerBound, value))
        withAnimation(reduceMotion ? nil : .easeInOut(duration: 0.15)) { zoom = next }
        pinchBase = next
    }

    // MARK: - 図

    private func map(_ graph: ScreenGraph) -> some View {
        GeometryReader { geo in
            let layout = ScreenMapLayout(graph: graph, availableWidth: geo.size.width / zoom)
            ScrollViewReader { proxy in
                ScrollView([.horizontal, .vertical]) {
                    MacScreenMapCanvas(graph: graph, layout: layout, hasTabs: dsl.app.hasTabs, selected: selected,
                                       select: { select($0, announce: false) }, open: onOpen, talk: talk)
                        .scaleEffect(zoom, anchor: .topLeading)
                        .frame(width: layout.contentWidth * zoom, height: layout.contentHeight * zoom, alignment: .topLeading)
                        .background(alignment: .topLeading) { anchors(graph, layout) }
                        .frame(minWidth: geo.size.width, minHeight: geo.size.height, alignment: .top)
                }
                .onChange(of: selected) { _, id in
                    guard let id else { return }
                    withAnimation(reduceMotion ? nil : .easeInOut(duration: 0.2)) { proxy.scrollTo(id) }
                }
            }
            .gesture(MagnifyGesture()
                .onChanged { value in zoom = min(Self.zoomRange.upperBound, max(Self.zoomRange.lowerBound, pinchBase * value.magnification)) }
                .onEnded { _ in pinchBase = zoom })
        }
        .background(Theme.Colors.canvas)
        // 矢印キー・Return は図が受ける(カードは 1 つずつ Tab で回らない。VoiceOver ではカードを 1 枚ずつ読む)
        .focusable()
        .focusEffectDisabled()
        .focused($mapFocused)
        .onKeyPress(.upArrow) { move(.up, graph) }
        .onKeyPress(.downArrow) { move(.down, graph) }
        .onKeyPress(.leftArrow) { move(.left, graph) }
        .onKeyPress(.rightArrow) { move(.right, graph) }
        .onKeyPress(.return) {
            guard let selected else { return .ignored }
            onOpen(selected)
            return .handled
        }
    }

    /// スクロールの行き先(拡大した後の置き場所)。`scaleEffect` は描くだけで場所を変えないので別に置く
    private func anchors(_ graph: ScreenGraph, _ layout: ScreenMapLayout) -> some View {
        ZStack(alignment: .topLeading) {
            ForEach(graph.nodes) { node in
                let f = layout.frame(of: node)
                Color.clear
                    .frame(width: f.width * zoom + 40, height: f.height * zoom + 40)
                    .id(node.id)
                    .offset(x: f.minX * zoom - 20, y: f.minY * zoom - 20)
            }
        }
        .accessibilityHidden(true)
    }

    private func move(_ direction: ScreenGraph.Move, _ graph: ScreenGraph) -> KeyPress.Result {
        guard let next = graph.neighbor(of: selected, direction) else { return .handled }
        select(next, announce: true)
        return .handled
    }

    private func select(_ id: String?, announce: Bool) {
        selected = id
        guard announce, let id, let node = graph.node(id) else { return }
        MacVoiceOver.announce(node.title)
    }

    private func talk(_ id: String) {
        guard let node = graph.node(id) else { return }
        onTalk(ScreenContext(id: node.id, title: node.title))
    }
}

/// カードと矢印。矢印は Canvas でカードの後ろに描く。カードは VoiceOver が上の段から読むよう段の順に並べる。
private struct MacScreenMapCanvas: View {
    let graph: ScreenGraph
    let layout: ScreenMapLayout
    let hasTabs: Bool
    let selected: String?
    let select: (String?) -> Void
    let open: (String) -> Void
    let talk: (String) -> Void

    var body: some View {
        ZStack(alignment: .topLeading) {
            Canvas { ctx, _ in layout.drawArrows(in: ctx, selected: selected, accent: Theme.Colors.brandA) }
                .frame(width: layout.contentWidth, height: layout.contentHeight)
                .allowsHitTesting(false)
                .accessibilityHidden(true)
            if layout.hasUnreachable {
                Label(L10n.tr("どこからも行けない画面"), systemImage: "exclamationmark.triangle")
                    .font(.caption.weight(.semibold))
                    .foregroundStyle(Theme.Colors.warning)
                    .offset(x: layout.inset, y: layout.rowTop(graph.rows.count - 1) - layout.unreachableLabel)
            }
            ForEach(graph.readingOrder, id: \.self) { id in
                if let node = graph.node(id) {
                    let f = layout.frame(of: node)
                    MacScreenCard(node: node, rootLabel: hasTabs ? L10n.tr("タブ") : L10n.tr("最初の画面"),
                                  isSelected: selected == node.id,
                                  select: { select(selected == node.id ? nil : node.id) },
                                  open: { open(node.id) }, talk: { talk(node.id) })
                        .frame(width: f.width, height: f.height)
                        .offset(x: f.minX, y: f.minY)
                }
            }
        }
        .frame(width: layout.contentWidth, height: layout.contentHeight, alignment: .topLeading)
        .contentShape(Rectangle())
        .onTapGesture { select(nil) } // 何もない所をクリックで選択を外す
    }
}

/// 画面 1 枚分のカード。タブの根・フォームはバッジで区別、到達できない画面は薄く破線枠(iOS の ScreenCard と同じ見た目)。
private struct MacScreenCard: View {
    let node: ScreenGraph.Node
    let rootLabel: String
    let isSelected: Bool
    let select: () -> Void
    let open: () -> Void
    let talk: () -> Void

    @State private var hovering = false

    var body: some View {
        face
            .contentShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
            .onHover { hovering = $0 }
            .onTapGesture(count: 2, perform: open)
            .onTapGesture(perform: select)
            .contextMenu {
                Button("この画面を開く", action: open)
                Button("この画面について話す", action: talk)
            }
            .help(node.title)
            .accessibilityElement(children: .ignore)
            .accessibilityLabel(Text(verbatim: "\(node.title)、\(badge)"))
            .accessibilityValue(accessibilityValue)
            .accessibilityAddTraits(isSelected ? [.isButton, .isSelected] : .isButton)
            .accessibilityHint("この画面を開きます")
            .accessibilityAction(.default, open)
            .accessibilityAction(named: Text("選ぶ"), select)
            .accessibilityAction(named: Text("この画面について話す"), talk)
            .accessibilityIdentifier("screenMap.node.\(node.id)")
    }

    private var accessibilityValue: Text {
        if !node.reachable { return Text("どこからも行けません") }
        return isSelected ? Text("選択中") : Text(verbatim: "")
    }

    private var face: some View {
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
                .font(.callout.weight(.semibold))
                .foregroundStyle(.primary)
                .lineLimit(2)
                .minimumScaleFactor(0.8)
        }
        .padding(.horizontal, 10)
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
        .background(Theme.Colors.surface, in: RoundedRectangle(cornerRadius: 12, style: .continuous))
        .overlay(
            RoundedRectangle(cornerRadius: 12, style: .continuous)
                .strokeBorder(border, style: StrokeStyle(lineWidth: isSelected ? 2.5 : 1, dash: node.reachable ? [] : [5, 3]))
        )
        .shadow(color: Theme.Shadow.softColor, radius: isSelected ? 10 : 4, y: 3)
        .opacity(node.reachable ? 1 : 0.5)
    }

    /// 選んでいればブランドの色・マウスが乗っていれば濃い線・それ以外は細い線
    private var border: AnyShapeStyle {
        if isSelected { return AnyShapeStyle(Theme.brandGradient) }
        return hovering ? AnyShapeStyle(.secondary) : AnyShapeStyle(Theme.Colors.hairline)
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

/// 右の列: 選んだ画面の出入り・「この画面を開く」「この画面について話す」。出入りの行を押すとその画面を選ぶ。
private struct MacScreenMapDetail: View {
    let graph: ScreenGraph
    let selected: String?
    let hasTabs: Bool
    /// 一覧の行から開く画面の、選べる記録(#1322)。nil なら記録を選ばない画面
    var recordChoices: [ScreenMapRecordChoice]?
    let open: (String) -> Void
    var openRecord: (String, String) -> Void = { _, _ in }
    let talk: (String) -> Void
    let select: (String) -> Void

    var body: some View {
        if let id = selected, let node = graph.node(id) {
            ScrollView {
                VStack(alignment: .leading, spacing: MacTheme.Spacing.group) {
                    VStack(alignment: .leading, spacing: 2) {
                        Text(node.title).font(.title3.weight(.semibold))
                        Text(node.id).font(.caption.monospaced()).foregroundStyle(.tertiary).textSelection(.enabled)
                        if !node.reachable {
                            Text("どこからも行けません")
                                .font(.caption.weight(.semibold))
                                .foregroundStyle(Theme.Colors.warning)
                        }
                    }
                    VStack(spacing: 8) {
                        Button { open(node.id) } label: {
                            Label("この画面を開く", systemImage: "arrow.up.forward.app").frame(maxWidth: .infinity)
                        }
                        .controlSize(.large)
                        .help("Return でも開けます")
                        recordMenu(node.id)
                        Button { talk(node.id) } label: {
                            Label(L10n.tr("この画面について話す"), systemImage: "bubble.left.and.text.bubble.right")
                                .frame(maxWidth: .infinity)
                        }
                        .controlSize(.large)
                        .buttonStyle(.borderedProminent)
                        .accessibilityIdentifier("screenMap.talk")
                    }
                    links(L10n.tr("ここから行ける"), graph.outgoing(from: node.id), outgoing: true)
                    links(L10n.tr("ここへ来られる"), graph.incoming(to: node.id), outgoing: false)
                }
                .padding(MacTheme.Spacing.window)
                .frame(maxWidth: .infinity, alignment: .leading)
            }
        } else {
            VStack(spacing: MacTheme.Spacing.group) {
                Image(systemName: "point.3.connected.trianglepath.dotted")
                    .font(.largeTitle)
                    .foregroundStyle(Theme.brandGradient)
                    .accessibilityHidden(true)
                Text("画面をクリックすると、出入りと「この画面について話す」が出ます")
                    .multilineTextAlignment(.center)
                    .foregroundStyle(.secondary)
                Text("矢印キーで選び、Return で開けます")
                    .font(.caption)
                    .multilineTextAlignment(.center)
                    .foregroundStyle(.tertiary)
            }
            .padding(MacTheme.Spacing.window)
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .accessibilityElement(children: .combine)
        }
    }

    /// 記録を選んで開く(#1322)。記録が無ければ、空で開くことを一言で知らせる
    @ViewBuilder private func recordMenu(_ screen: String) -> some View {
        if let choices = recordChoices {
            if choices.isEmpty {
                Text("記録がまだ無いので、空のまま開きます")
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .frame(maxWidth: .infinity, alignment: .leading)
            } else {
                Menu {
                    ForEach(choices.prefix(ScreenMapRecordChoice.menuLimit)) { choice in
                        Button(choice.title) { openRecord(screen, choice.id) }
                    }
                } label: {
                    Label("記録を選んで開く", systemImage: "list.bullet.rectangle").frame(maxWidth: .infinity)
                }
                .controlSize(.large)
                .help("どの記録でこの画面を開くかを選びます")
                .accessibilityIdentifier("screenMap.openRecord")
            }
        }
    }

    private func links(_ label: String, _ links: [ScreenGraph.Link], outgoing: Bool) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(label).font(.caption.weight(.semibold)).foregroundStyle(.secondary)
            if links.isEmpty {
                Text("なし").font(.callout).foregroundStyle(.tertiary)
            }
            ForEach(links) { link in
                let otherId = outgoing ? link.to : link.from
                let other = graph.node(otherId)?.title ?? otherId
                let how = link.edges.map(ScreenMapCopy.describe).joined(separator: L10n.tr("、"))
                Button { select(otherId) } label: {
                    HStack(alignment: .firstTextBaseline, spacing: 6) {
                        Image(systemName: outgoing ? "arrow.right" : "arrow.left")
                            .font(.caption.weight(.bold))
                            .foregroundStyle(Theme.brandGradient)
                        VStack(alignment: .leading, spacing: 1) {
                            Text(other).font(.callout.weight(.semibold))
                            Text(how).font(.caption).foregroundStyle(.secondary).lineLimit(2)
                        }
                    }
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .help("この画面を選ぶ")
                .accessibilityLabel(Text(verbatim: "\(other)、\(how)"))
                .accessibilityHint("この画面を選びます")
            }
        }
    }
}

/// 画面マップで選べる記録 1 件(#1322)。title は一覧の見出しと同じ(見出しの無い記録は「記録 n」)
struct ScreenMapRecordChoice: Identifiable, Equatable {
    /// メニューに並べる数の上限(多すぎる時は一覧の先頭から)
    static let menuLimit = 50

    let id: String
    let title: String
}
