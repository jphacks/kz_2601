import SwiftUI
import MetamonDSL

/// `bracket`(トーナメント表): entity の出場者から組み合わせを作り、回戦を列にして勝ち上がりを出す。
/// 試合の 1 人をタップするとその人を勝者にし(もう一度で取り消し)、winsField に保存する(`list.item.toggle` と同じ `updateRecord`・楽観的 UI)。
/// 記録を変更できない人のタップは `notifyReadOnly`。組み合わせと勝ち上がりの計算は `BracketDraw`(MetamonDSL)。
struct BracketLeaf: View {
    @Environment(\.dslRuntime) private var rt
    let c: Component
    let ctx: RenderContext

    /// 保存待ちの勝ち数(recordId → wins)
    @State private var pending: [String: Int] = [:]
    @State private var taps = 0

    private var entityId: String { c.entity ?? "" }
    private var entity: Entity? { rt.dsl.entity(entityId) }
    /// 1 回戦の 1 試合の高さ。後の回戦は 2 倍ずつ(線で結ばずに、上がる先の試合が真ん中に来る)
    private let unit: CGFloat = 76
    private let columnWidth: CGFloat = 150

    var body: some View {
        let draw = Self.draw(c, rt: rt, pending: pending)
        VStack(alignment: .leading, spacing: RenderTheme.Spacing.sm) {
            header(draw)
            if draw.rounds.isEmpty {
                if draw.entrants.count == 1 {
                    Text(L("出場者が 2 人以上になると表ができます"))
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                        .frame(maxWidth: .infinity, maxHeight: .infinity)
                } else {
                    EmptyPlaceholder(icon: "trophy", emptyText: L("出場者を追加してください"), state: rt.emptyState(for: entityId)) {
                        Task { await rt.load(entities: [entityId], force: true) }
                    }
                }
            } else {
                ScrollView([.horizontal, .vertical]) {
                    HStack(alignment: .top, spacing: RenderTheme.Spacing.sm) {
                        ForEach(Array(draw.rounds.enumerated()), id: \.offset) { i, matches in
                            column(matches, round: i + 1, total: draw.rounds.count)
                        }
                    }
                    .padding(.bottom, RenderTheme.Spacing.xs)
                }
                .accessibilityIdentifier("bracket.rounds")
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        .renderCard(wash: true)
        .sensoryFeedback(.selection, trigger: taps)
    }

    @ViewBuilder
    private func header(_ draw: BracketDraw) -> some View {
        HStack(spacing: RenderTheme.Spacing.sm) {
            if let title = c.title, !title.isEmpty {
                Text(title)
                    .font(RenderTheme.Fonts.sectionTitle)
                    .foregroundStyle(.secondary)
                    .accessibilityAddTraits(.isHeader)
            }
            Spacer(minLength: 0)
            if let champ = draw.champion {
                Label(L("優勝: \(name(champ))"), systemImage: "trophy.fill")
                    .font(.subheadline.weight(.semibold))
                    .foregroundStyle(RenderTheme.brandFill)
                    .accessibilityIdentifier("bracket.champion")
            }
        }
    }

    private func column(_ matches: [BracketDraw.Match], round: Int, total: Int) -> some View {
        let slot = unit * CGFloat(1 << (round - 1))
        return VStack(spacing: 0) {
            Text(Self.roundTitle(round, of: total))
                .font(.caption.weight(.semibold))
                .foregroundStyle(.secondary)
                .frame(height: 20)
                .accessibilityAddTraits(.isHeader)
            ForEach(matches, id: \.index) { m in
                matchCard(m)
                    .frame(height: slot)
            }
        }
        .frame(width: columnWidth)
    }

    private func matchCard(_ m: BracketDraw.Match) -> some View {
        VStack(spacing: 0) {
            row(m, side: .a)
            Divider()
            row(m, side: .b)
        }
        .background(RenderTheme.Colors.brandSoft.opacity(0.5), in: RoundedRectangle(cornerRadius: 10))
        .overlay(RoundedRectangle(cornerRadius: 10).stroke(RenderTheme.Colors.brandSoftStroke))
        .padding(.vertical, RenderTheme.Spacing.xs)
    }

    private func row(_ m: BracketDraw.Match, side: BracketDraw.Side) -> some View {
        let r = side == .a ? m.a : m.b
        let won = m.winner == side
        let lost = m.winner != nil && !won
        let text = r.map(name) ?? (m.round == 1 ? L("不戦勝") : L("未定"))
        return Button {
            tap(m, side: side)
        } label: {
            HStack(spacing: RenderTheme.Spacing.xs) {
                Text(text)
                    .font(.subheadline.weight(won ? .bold : .regular))
                    .foregroundStyle(r == nil || lost ? AnyShapeStyle(Color.secondary) : AnyShapeStyle(Color.primary))
                    .lineLimit(1)
                    .minimumScaleFactor(0.7)
                Spacer(minLength: 0)
                if won {
                    Image(systemName: "checkmark.circle.fill")
                        .foregroundStyle(RenderTheme.brandFill)
                        .accessibilityHidden(true)
                }
            }
            .padding(.horizontal, RenderTheme.Spacing.sm)
            .frame(maxWidth: .infinity, minHeight: 30, alignment: .leading)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .disabled(!m.isPlayable)
        .accessibilityLabel(text)
        .accessibilityValue(won ? L("勝ち") : "")
        .accessibilityHint(m.isPlayable ? L("ダブルタップで勝者にする") : "")
        .accessibilityIdentifier("bracket.r\(m.round).m\(m.index).\(side.rawValue)")
    }

    private func name(_ r: Record) -> String {
        guard let f = c.titleField, !f.isEmpty else { return r.id }
        let s = rt.display(r[f], field: entity?.field(f))
        return s.isEmpty ? "—" : s
    }

    // MARK: - 勝者を選ぶ

    private func tap(_ m: BracketDraw.Match, side: BracketDraw.Side) {
        guard let winsField = Self.winsField(c, in: entity) else { return }
        switch BracketDraw.tap(m, side: side, winsField: winsField, pending: pending) {
        case .notPlayable:
            return
        case .locked:
            AccessibilityNotification.Announcement(L("先の回戦の結果を取り消してから変えてください")).post()
            return
        case .update(let wins):
            let records = rt.records(for: entityId)
            for id in wins.keys {
                if let r = records.first(where: { $0.id == id }), !rt.canEdit(entity: entityId, record: r) {
                    rt.notifyReadOnly(entity: entityId)
                    return
                }
            }
            for (id, w) in wins { pending[id] = w }
            taps += 1
            for (id, w) in wins {
                Task {
                    let saved = await rt.updateRecord(entity: entityId, recordId: id, data: [winsField: .number(Double(w))])
                    if !saved { print("[Metamon] bracket not saved: \(entityId)/\(id)") }
                    if pending[id] == w { pending[id] = nil }
                }
            }
        }
    }

    // MARK: - 計算(描画の木 RenderTree と共用)

    /// 勝ち数を保存できる field(integer・computed でない)。条件を満たさなければ nil(表示だけ)。
    static func winsField(_ c: Component, in entity: Entity?) -> String? {
        guard let id = c.winsField, let f = entity?.field(id), f.computed == nil, f.type == .integer else { return nil }
        return id
    }

    static func draw(_ c: Component, rt: any DSLRuntime, pending: [String: Int]) -> BracketDraw {
        BracketDraw(records: rt.records(for: c.entity ?? ""), winsField: c.winsField ?? "", seedField: c.seedField, pending: pending)
    }

    /// 決勝・準決勝・準々決勝、それより前は「n 回戦」。
    static func roundTitle(_ round: Int, of total: Int) -> String {
        switch total - round {
        case 0: L("決勝")
        case 1: L("準決勝")
        case 2: L("準々決勝")
        default: L("\(round) 回戦")
        }
    }
}
