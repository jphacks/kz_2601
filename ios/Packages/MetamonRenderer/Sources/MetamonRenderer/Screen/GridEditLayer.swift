import SwiftUI
import MetamonDSL

/// 手で動かす編集モード(#1442)の状態。ホストが作って `\.gridEditSession` で渡し、「完了」で `draft.patch` を保存する。
/// v1 は Screen 直下で `area` を持つ部品の移動・大きさ・削除だけ(追加・文言・Container の中の並べ替えは後で)。
@MainActor
@Observable
public final class GridEditSession {
    public private(set) var draft: GridEditDraft
    /// 指で動かしている間は画面のスクロールを止める(ドラッグがスクロールに取られない)
    public internal(set) var isDragging = false
    /// 消す確認を出している部品
    var pendingRemoval: String?

    public init(draft: GridEditDraft) { self.draft = draft }

    public var screenId: String { draft.screenId }

    @discardableResult
    func place(_ id: String, at area: GridArea) -> Bool { draft.place(id, at: area) }

    func remove(_ id: String) { draft.remove(id) }
}

private struct GridEditSessionKey: EnvironmentKey {
    static let defaultValue: GridEditSession? = nil
}

public extension EnvironmentValues {
    /// 編集モード中の画面(#1442)。nil なら普段どおり描く
    var gridEditSession: GridEditSession? {
        get { self[GridEditSessionKey.self] }
        set { self[GridEditSessionKey.self] = newValue }
    }
}

/// 編集できる画面か。フォーム画面は配置が中身なり(`FormFlowLayout`)・音声だけの画面はグリッドの意味が違うので対象外。
public extension Screen {
    var isGridEditable: Bool { entity == nil && !isVoiceScreen && !gridPlacement.placed.isEmpty }
}

/// マス目の線(編集中だけ下に敷く)。
struct GridEditLines: View {
    let cellW: CGFloat
    let cellH: CGFloat
    let rows: Int

    var body: some View {
        Canvas { ctx, size in
            var path = Path()
            for c in 0...GridArea.columns {
                let x = CGFloat(c) * cellW
                path.move(to: CGPoint(x: x, y: 0))
                path.addLine(to: CGPoint(x: x, y: size.height))
            }
            for r in 0...rows {
                let y = CGFloat(r) * cellH
                path.move(to: CGPoint(x: 0, y: y))
                path.addLine(to: CGPoint(x: size.width, y: y))
            }
            ctx.stroke(path, with: .color(RenderTheme.Colors.brandA.opacity(0.18)), style: StrokeStyle(lineWidth: 1, dash: [3, 3]))
        }
        .frame(width: cellW * CGFloat(GridArea.columns), height: cellH * CGFloat(rows))
        .allowsHitTesting(false)
        .accessibilityHidden(true)
    }
}

/// 部品の枠・`area` の札・角のつまみ・✕。ドラッグ中はマス目に吸着した置き先を出し、重なる時は赤くして置かせない。
struct GridEditLayer: View {
    @Bindable var session: GridEditSession
    let cellW: CGFloat
    let cellH: CGFloat
    let gap: CGFloat

    private enum Mode { case move, resize }
    private struct Drag: Equatable {
        var id: String
        var candidate: GridArea
        var translation: CGSize
        var moving: Bool
    }
    @State private var drag: Drag?

    private var conflicts: Set<String> {
        guard let drag else { return [] }
        return Set(session.draft.conflicts(for: drag.id, at: drag.candidate))
    }

    var body: some View {
        let blocked = conflicts
        ZStack(alignment: .topLeading) {
            if let drag {
                // 置き先(吸着した矩形)
                RoundedRectangle(cornerRadius: RenderTheme.Radius.card, style: .continuous)
                    .fill((blocked.isEmpty ? RenderTheme.Colors.brandA : Color.red).opacity(0.14))
                    .overlay(
                        RoundedRectangle(cornerRadius: RenderTheme.Radius.card, style: .continuous)
                            .strokeBorder(blocked.isEmpty ? RenderTheme.Colors.brandA : Color.red, style: StrokeStyle(lineWidth: 2, dash: [6, 4]))
                    )
                    .overlay(alignment: .center) {
                        if !blocked.isEmpty {
                            Text(L("ここには置けません"))
                                .font(.caption.weight(.semibold))
                                .foregroundStyle(.white)
                                .padding(.horizontal, 8).padding(.vertical, 4)
                                .background(Color.red, in: Capsule())
                        }
                    }
                    .frame(width: rect(drag.candidate).width, height: rect(drag.candidate).height)
                    .offset(x: rect(drag.candidate).minX, y: rect(drag.candidate).minY)
                    .allowsHitTesting(false)
            }
            ForEach(session.draft.placed) { item in
                outline(item, conflicting: blocked.contains(item.componentId))
            }
        }
        .confirmationDialog(
            L("この部品を消しますか?"),
            isPresented: Binding(get: { session.pendingRemoval != nil }, set: { if !$0 { session.pendingRemoval = nil } }),
            titleVisibility: .visible
        ) {
            Button(L("消す"), role: .destructive) {
                if let id = session.pendingRemoval { withAnimation(.easeOut(duration: 0.2)) { session.remove(id) } }
                session.pendingRemoval = nil
            }
            Button(L("キャンセル"), role: .cancel) { session.pendingRemoval = nil }
        } message: {
            Text(L("「完了」を押すと保存されます。保存した後も「元に戻す」で戻せます。"))
        }
        .sensoryFeedback(.warning, trigger: blocked.isEmpty) { wasEmpty, isEmpty in wasEmpty && !isEmpty }
    }

    private func rect(_ a: GridArea) -> CGRect {
        CGRect(x: CGFloat(a.col1) * cellW + gap / 2, y: CGFloat(a.row1 - 1) * cellH + gap / 2,
               width: CGFloat(a.colSpan) * cellW - gap, height: CGFloat(a.rowSpan) * cellH - gap)
    }

    @ViewBuilder
    private func outline(_ item: GridEditDraft.Item, conflicting: Bool) -> some View {
        let r = rect(item.area)
        let active = drag?.id == item.componentId
        let follow = active && drag?.moving == true ? drag!.translation : .zero
        let tint: Color = conflicting ? .red : RenderTheme.Colors.brandA
        RoundedRectangle(cornerRadius: RenderTheme.Radius.card, style: .continuous)
            .fill(tint.opacity(active ? 0.12 : 0.04))
            .overlay(
                RoundedRectangle(cornerRadius: RenderTheme.Radius.card, style: .continuous)
                    .strokeBorder(tint, lineWidth: active || conflicting ? 2.5 : 1.5)
            )
            .overlay(alignment: .topLeading) {
                Text(active ? drag!.candidate.notation : item.area.notation)
                    .font(.caption2.monospaced().weight(.semibold))
                    .foregroundStyle(.white)
                    .padding(.horizontal, 6).padding(.vertical, 2)
                    .background(tint, in: Capsule())
                    .padding(4)
                    .accessibilityHidden(true)
            }
            .contentShape(Rectangle())
            .highPriorityGesture(dragGesture(item, mode: .move))
            .overlay(alignment: .topTrailing) {
                if session.draft.canRemove {
                    Button { session.pendingRemoval = item.componentId } label: {
                        Image(systemName: "xmark")
                            .font(.caption.weight(.bold))
                            .foregroundStyle(.white)
                            .frame(width: 24, height: 24)
                            .background(Color.red, in: Circle())
                            .frame(width: 44, height: 44) // 押しやすい大きさ
                            .contentShape(Rectangle())
                    }
                    .buttonStyle(.plain)
                    .offset(x: 12, y: -12)
                    .accessibilityLabel(Text(L("消す")))
                }
            }
            .overlay(alignment: .bottomTrailing) {
                Image(systemName: "arrow.up.left.and.arrow.down.right")
                    .font(.caption2.weight(.bold))
                    .foregroundStyle(.white)
                    .frame(width: 24, height: 24)
                    .background(tint, in: Circle())
                    .frame(width: 44, height: 44)
                    .contentShape(Rectangle())
                    .offset(x: 10, y: 10)
                    .highPriorityGesture(dragGesture(item, mode: .resize))
                    .accessibilityHidden(true)
            }
            .frame(width: r.width, height: r.height)
            .offset(x: r.minX + follow.width, y: r.minY + follow.height)
            .zIndex(active ? 1 : 0)
            .accessibilityElement(children: .contain)
            .accessibilityLabel(Text(L("部品 \(item.area.notation)")))
            .accessibilityActions {
                Button(L("左へ動かす")) { nudge(item, dc: -1, dr: 0) }
                Button(L("右へ動かす")) { nudge(item, dc: 1, dr: 0) }
                Button(L("上へ動かす")) { nudge(item, dc: 0, dr: -1) }
                Button(L("下へ動かす")) { nudge(item, dc: 0, dr: 1) }
            }
            .accessibilityIdentifier("gridEdit.\(item.componentId)")
    }

    private func dragGesture(_ item: GridEditDraft.Item, mode: Mode) -> some Gesture {
        DragGesture(minimumDistance: 2)
            .onChanged { v in
                let t = v.translation
                let candidate = mode == .move
                    ? GridEditMath.moved(item.area, dx: Double(t.width), dy: Double(t.height), cellW: Double(cellW), cellH: Double(cellH))
                    : GridEditMath.resized(item.area, dx: Double(t.width), dy: Double(t.height), cellW: Double(cellW), cellH: Double(cellH))
                session.isDragging = true
                drag = Drag(id: item.componentId, candidate: candidate, translation: t, moving: mode == .move)
            }
            .onEnded { _ in
                if let d = drag, d.id == item.componentId {
                    withAnimation(.spring(duration: 0.3, bounce: 0.15)) {
                        _ = session.place(d.id, at: d.candidate) // 重なる時は置かずに元へ戻る
                        drag = nil
                    }
                }
                session.isDragging = false
            }
    }

    private func nudge(_ item: GridEditDraft.Item, dc: Int, dr: Int) {
        let moved = GridEditMath.moved(item.area, dx: Double(dc) * Double(cellW), dy: Double(dr) * Double(cellH),
                                       cellW: Double(cellW), cellH: Double(cellH))
        if !session.place(item.componentId, at: moved) {
            AccessibilityNotification.Announcement(L("ここには置けません")).post()
        }
    }
}
