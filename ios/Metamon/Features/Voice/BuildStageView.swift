import SwiftUI
import MetamonDSL

/// 生成中の「組み立て台」(#626)。上に空の画面(6 列 × 12 行)、下にパーツ箱。
/// - 待っている間: `WaitingScript` のコマに合わせて、箱の部品が持ち上がり → 画面に置かれる・入れ替わる・並べ直される
/// - 結果が届いた後(`plan` あり): 本物の結果の部品を上から 1 つずつ置く
/// Reduce Motion では位置を動かさず、フェードだけで切り替える。VoiceOver からは隠す(状態は GeneratingView の文言で伝える)。
struct BuildStageView: View {
    let script: BuildChoreography.WaitingScript
    let elapsed: TimeInterval
    /// 結果の組み上げ(nil なら待っている間)
    var plan: BuildChoreography.AssemblyPlan?
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    /// 組み上げの開始(この時刻からの経過で各部品の delay を見る)
    @State private var revealStart: Date?

    private var waitingFrame: BuildChoreography.WaitingFrame {
        script.frame(at: BuildChoreography.WaitingScript.step(elapsed: elapsed, reduceMotion: reduceMotion))
    }

    var body: some View {
        VStack(spacing: Theme.Spacing.md) {
            if let plan {
                TimelineView(.animation(minimumInterval: 1.0 / 30, paused: reduceMotion)) { tl in
                    let t = revealStart.map { tl.date.timeIntervalSince($0) } ?? 0
                    let placed = plan.steps.filter { reduceMotion || $0.delay <= t }.map(\.part)
                    let picking = plan.steps.first { !reduceMotion && $0.delay > t }?.part.kind
                    stage(parts: placed, rows: plan.rows, tray: plan.tray, picking: picking)
                }
                .onAppear { if revealStart == nil { revealStart = .now } }
            } else {
                let frame = waitingFrame
                stage(parts: frame.placed, rows: GridArea.visibleRows, tray: script.tray, picking: frame.picking)
            }
        }
        .accessibilityHidden(true)
    }

    private func stage(parts: [BuildChoreography.PlacedPart], rows: Int, tray: [BuildChoreography.PartKind],
                       picking: BuildChoreography.PartKind?) -> some View {
        VStack(spacing: Theme.Spacing.md) {
            BuildCanvas(parts: parts, rows: rows, reduceMotion: reduceMotion)
                .frame(maxWidth: 220)
                .aspectRatio(6.0 / 10.0, contentMode: .fit)
            PartTray(kinds: tray, picking: picking, reduceMotion: reduceMotion)
        }
    }
}

/// 空の画面に部品のシルエットを置く。部品の id が同じなら場所の変化をアニメーションで動かす(並べ直し)。
private struct BuildCanvas: View {
    let parts: [BuildChoreography.PlacedPart]
    let rows: Int
    let reduceMotion: Bool

    private let gap: CGFloat = 5

    var body: some View {
        GeometryReader { geo in
            let inset: CGFloat = 8
            let w = max(0, geo.size.width - inset * 2), h = max(0, geo.size.height - inset * 2)
            let cw = w / CGFloat(GridArea.columns)
            // 12 行を超える結果は縦を詰めて全部を収める(全体像が見えるように)
            let ch = h / CGFloat(max(rows, GridArea.visibleRows))
            ZStack(alignment: .topLeading) {
                // 画面の地(ドットのグリッド)
                ForEach(0..<(GridArea.columns * GridArea.visibleRows), id: \.self) { i in
                    let c = i % GridArea.columns, r = i / GridArea.columns
                    Circle()
                        .fill(Theme.Colors.brandA.opacity(0.18))
                        .frame(width: 3, height: 3)
                        .offset(x: CGFloat(c) * cw + cw / 2 - 1.5,
                                y: CGFloat(r) * (h / CGFloat(GridArea.visibleRows)) + h / CGFloat(GridArea.visibleRows) / 2 - 1.5)
                }
                ForEach(parts) { p in
                    PartSilhouette(kind: p.kind, caption: p.caption)
                        .frame(width: max(0, CGFloat(p.area.colSpan) * cw - gap), height: max(0, CGFloat(p.area.rowSpan) * ch - gap))
                        .offset(x: CGFloat(p.area.col1) * cw + gap / 2, y: CGFloat(p.area.row1 - 1) * ch + gap / 2)
                        .transition(reduceMotion ? .opacity : .asymmetric(
                            insertion: .scale(scale: 0.7, anchor: .bottom).combined(with: .opacity).combined(with: .offset(y: 24)),
                            removal: .scale(scale: 0.85).combined(with: .opacity)))
                }
            }
            .padding(inset)
            .animation(reduceMotion ? .easeInOut(duration: 0.4) : .spring(duration: 0.45, bounce: 0.25), value: parts)
        }
        .background(Theme.Colors.surface.opacity(0.7), in: RoundedRectangle(cornerRadius: Theme.Radius.md, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: Theme.Radius.md, style: .continuous).strokeBorder(Theme.Colors.hairline))
        .shadow(color: Theme.Shadow.softColor, radius: 12, y: 6)
    }
}

/// パーツ箱。今選んでいる部品が持ち上がって光る。
private struct PartTray: View {
    let kinds: [BuildChoreography.PartKind]
    let picking: BuildChoreography.PartKind?
    let reduceMotion: Bool

    var body: some View {
        HStack(spacing: Theme.Spacing.sm) {
            ForEach(kinds, id: \.self) { kind in
                let active = kind == picking
                Image(systemName: kind.symbol)
                    .font(.footnote.weight(.semibold))
                    .foregroundStyle(active ? AnyShapeStyle(.white) : AnyShapeStyle(Theme.Colors.brandA.opacity(0.8)))
                    .frame(width: 34, height: 34)
                    .background(
                        RoundedRectangle(cornerRadius: 9, style: .continuous)
                            .fill(active ? AnyShapeStyle(Theme.brandGradient) : AnyShapeStyle(Theme.Colors.surface))
                    )
                    .overlay(RoundedRectangle(cornerRadius: 9, style: .continuous).strokeBorder(Theme.Colors.hairline))
                    .scaleEffect(active && !reduceMotion ? 1.15 : 1)
                    .offset(y: active && !reduceMotion ? -6 : 0)
                    .shadow(color: active ? Theme.Colors.brandB.opacity(0.4) : .clear, radius: 6, y: 3)
            }
        }
        .padding(.horizontal, Theme.Spacing.sm)
        .padding(.vertical, Theme.Spacing.sm)
        .background(.thinMaterial, in: Capsule())
        .animation(reduceMotion ? .easeInOut(duration: 0.3) : .spring(duration: 0.35, bounce: 0.35), value: picking)
        .animation(.easeInOut(duration: 0.35), value: kinds)
    }
}

/// 実在する部品の形のシルエット。本物の結果では部品の名前(caption)を添える。
struct PartSilhouette: View {
    let kind: BuildChoreography.PartKind
    var caption: String?

    private var tint: Color { Theme.Colors.brandA }

    var body: some View {
        ZStack {
            RoundedRectangle(cornerRadius: 6, style: .continuous)
                .fill(kind == .button ? AnyShapeStyle(Theme.brandGradient.opacity(0.85)) : AnyShapeStyle(Theme.Colors.surfaceElevated))
            RoundedRectangle(cornerRadius: 6, style: .continuous)
                .strokeBorder(Theme.Colors.brandB.opacity(kind == .button ? 0 : 0.35), lineWidth: 1)
            content.padding(4)
        }
    }

    @ViewBuilder private var content: some View {
        switch kind {
        case .heading:
            VStack(alignment: .leading, spacing: 3) {
                label(bold: true) ?? AnyView(bar(0.7, height: 6))
                bar(0.4, height: 3)
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
        case .text:
            VStack(alignment: .leading, spacing: 3) {
                label() ?? AnyView(bar(0.8))
                bar(0.6)
                bar(0.7)
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        case .button:
            Group {
                if let caption {
                    Text(caption).font(.system(size: 8, weight: .semibold)).foregroundStyle(.white).lineLimit(1)
                } else {
                    Capsule().fill(.white.opacity(0.85)).frame(width: 28, height: 3)
                }
            }
        case .field:
            VStack(alignment: .leading, spacing: 3) {
                label() ?? AnyView(bar(0.35, height: 3))
                RoundedRectangle(cornerRadius: 3).strokeBorder(tint.opacity(0.4)).frame(maxHeight: 14)
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        case .toggle:
            HStack(spacing: 4) {
                Image(systemName: "checkmark.square.fill").font(.system(size: 9)).foregroundStyle(tint.opacity(0.7))
                label() ?? AnyView(bar(0.5))
                Spacer(minLength: 0)
                Capsule().fill(tint.opacity(0.5)).frame(width: 14, height: 8)
            }
        case .list:
            VStack(spacing: 4) {
                ForEach(0..<3, id: \.self) { _ in
                    HStack(spacing: 3) {
                        Circle().fill(tint.opacity(0.35)).frame(width: 6, height: 6)
                        bar(0.75)
                    }
                }
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
        case .chart:
            HStack(alignment: .bottom, spacing: 3) {
                ForEach([0.4, 0.7, 0.5, 0.9, 0.6], id: \.self) { v in
                    RoundedRectangle(cornerRadius: 1.5).fill(Theme.brandGradient.opacity(0.7))
                        .frame(maxHeight: .infinity, alignment: .bottom)
                        .scaleEffect(y: v, anchor: .bottom)
                }
            }
        case .stat:
            VStack(alignment: .leading, spacing: 3) {
                label() ?? AnyView(bar(0.4, height: 3))
                Text(verbatim: "123").font(.system(size: 11, weight: .bold, design: .rounded)).foregroundStyle(tint.opacity(0.7))
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
        case .calendar:
            Grid(horizontalSpacing: 2, verticalSpacing: 2) {
                ForEach(0..<3, id: \.self) { _ in
                    GridRow { ForEach(0..<7, id: \.self) { _ in Circle().fill(tint.opacity(0.3)).frame(width: 3, height: 3) } }
                }
            }
        case .map:
            Image(systemName: "mappin.and.ellipse").font(.system(size: 12)).foregroundStyle(tint.opacity(0.7))
        case .media:
            Image(systemName: "photo").font(.system(size: 12)).foregroundStyle(tint.opacity(0.7))
        case .card:
            VStack(alignment: .leading, spacing: 3) {
                label() ?? AnyView(bar(0.6))
                bar(0.8)
                bar(0.5)
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        }
    }

    private func bar(_ width: CGFloat, height: CGFloat = 2.5) -> some View {
        GeometryReader { g in
            Capsule().fill(tint.opacity(0.3)).frame(width: g.size.width * width, height: height)
        }
        .frame(height: height)
    }

    private func label(bold: Bool = false) -> AnyView? {
        guard let caption else { return nil }
        return AnyView(Text(caption)
            .font(.system(size: bold ? 9 : 7, weight: bold ? .bold : .medium))
            .foregroundStyle(.secondary)
            .lineLimit(1))
    }
}

extension BuildChoreography.PartKind {
    /// パーツ箱のアイコン(SF Symbols)
    var symbol: String {
        switch self {
        case .heading: "textformat.size"
        case .text: "text.alignleft"
        case .button: "hand.tap"
        case .field: "character.cursor.ibeam"
        case .toggle: "checkmark.square"
        case .list: "list.bullet"
        case .chart: "chart.bar.fill"
        case .stat: "number"
        case .calendar: "calendar"
        case .map: "map"
        case .media: "photo"
        case .card: "square.on.square"
        }
    }
}

/// 編集の待ち(#626): カプセルの左に置く小さな部品の並べ直し。3 つの部品が順に入れ替わる。Reduce Motion では回転しない。
struct EditShuffleGlyph: View {
    let elapsed: TimeInterval
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    private static let kinds: [BuildChoreography.PartKind] = [.heading, .field, .list, .button, .chart]

    var body: some View {
        let step = reduceMotion ? 0 : Int(elapsed / 0.7)
        let shown = (0..<3).map { Self.kinds[(step + $0) % Self.kinds.count] }
        VStack(spacing: 2) {
            ForEach(Array(shown.enumerated()), id: \.element) { i, kind in
                RoundedRectangle(cornerRadius: 2, style: .continuous)
                    .fill(i == 0 ? AnyShapeStyle(Theme.brandGradient) : AnyShapeStyle(Theme.Colors.brandA.opacity(0.3)))
                    .frame(width: i == 1 ? 12 : 18, height: 4)
                    .frame(width: 18, alignment: .leading)
            }
        }
        .frame(width: 20, height: 20)
        .animation(.spring(duration: 0.4, bounce: 0.3), value: step)
        .accessibilityHidden(true)
    }
}
