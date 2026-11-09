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
    /// 1 回戦の 1 試合の高さ。後の回戦は 2 倍ずつ(線で結ばずに、上がる先の試合が真ん中に来る)。文字の大きさに合わせて伸ばす。
    @ScaledMetric(relativeTo: .subheadline) private var unit: CGFloat = 76
    @ScaledMetric(relativeTo: .subheadline) private var columnWidth: CGFloat = 150

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
                            column(matches, round: i + 1, draw: draw)
                        }
                    }
                    .padding(.bottom, RenderTheme.Spacing.xs)
                }
                .accessibilityIdentifier("bracket.rounds")
            }
            if draw.lateEntrants > 0 {
                Text(L("開始後に追加された \(draw.lateEntrants) 人は、この表に入りません"))
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .accessibilityIdentifier("bracket.late")
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

    private func column(_ matches: [BracketDraw.Match], round: Int, draw: BracketDraw) -> some View {
        let slot = unit * CGFloat(1 << (round - 1))
        let title = Self.roundTitle(round, of: draw.rounds.count)
        return VStack(spacing: 0) {
            Text(title)
                .font(.caption.weight(.semibold))
                .foregroundStyle(.secondary)
                .frame(height: 20)
                .accessibilityAddTraits(.isHeader)
            ForEach(matches, id: \.index) { m in
                matchCard(m, draw: draw)
                    .frame(height: slot)
                    .accessibilityElement(children: .contain)
                    .accessibilityLabel(Self.matchLabel(m, roundTitle: title))
            }
        }
        .frame(width: columnWidth)
    }

    private func matchCard(_ m: BracketDraw.Match, draw: BracketDraw) -> some View {
        VStack(spacing: 0) {
            row(m, side: .a, draw: draw)
            Divider()
            row(m, side: .b, draw: draw)
        }
        .background(RenderTheme.Colors.brandSoft.opacity(0.5), in: RoundedRectangle(cornerRadius: 10))
        .overlay(RoundedRectangle(cornerRadius: 10).stroke(RenderTheme.Colors.brandSoftStroke))
        .padding(.vertical, RenderTheme.Spacing.xs)
    }

    private func row(_ m: BracketDraw.Match, side: BracketDraw.Side, draw: BracketDraw) -> some View {
        let r = side == .a ? m.a : m.b
        let won = m.winner == side
        let lost = m.winner != nil && !won
        let text = slotText(m, side: side)
        let opponent = slotText(m, side: side == .a ? .b : .a)
        let locked = m.isPlayable && m.winner != nil && draw.nextDecided(m)
        return Button {
            tap(m, side: side, draw: draw)
        } label: {
            HStack(spacing: RenderTheme.Spacing.xs) {
                Text(text)
                    .font(.subheadline.weight(won ? .bold : .regular))
                    .foregroundStyle(r == nil || lost ? AnyShapeStyle(Color.secondary) : AnyShapeStyle(Color.primary))
                    .lineLimit(2)
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
        .accessibilityLabel(L("\(text)(相手: \(opponent))"))
        .accessibilityValue(won ? L("勝ち") : (lost ? L("負け") : ""))
        .accessibilityHint(!m.isPlayable || locked ? "" : L("ダブルタップで勝者にする"))
        .accessibilityIdentifier("bracket.r\(m.round).m\(m.index).\(side.rawValue)")
    }

    private func slotText(_ m: BracketDraw.Match, side: BracketDraw.Side) -> String {
        (side == .a ? m.a : m.b).map(name) ?? (m.round == 1 ? L("不戦勝") : L("未定"))
    }

    private func name(_ r: Record) -> String {
        guard let f = c.titleField, !f.isEmpty else { return r.id }
        let s = rt.display(r[f], field: entity?.field(f))
        return s.isEmpty ? "—" : s
    }

    // MARK: - 勝者を選ぶ

    private func tap(_ m: BracketDraw.Match, side: BracketDraw.Side, draw: BracketDraw) {
        guard let winsField = Self.winsField(c, in: entity) else { return }
        switch draw.tap(m, side: side, winsField: winsField, pending: pending) {
        case .notPlayable:
            return
        case .locked:
            AccessibilityNotification.Announcement(L("先の回戦の結果を取り消してから変えてください")).post()
            return
        case .update(let wins):
            let records = rt.records(for: entityId)
            let started = Self.startedField(c, in: entity)
            let starts = draw.startWrites(for: wins, records: records, winsField: winsField, startedField: started, pending: pending,
                                          now: FieldCoding.datetimeFormatter.string(from: DSLClock.now()))
            let plan = Self.writePlan(wins: wins, starts: starts, winsField: winsField, startedField: started)
            for (id, _) in plan {
                if let r = records.first(where: { $0.id == id }), !rt.canEdit(entity: entityId, record: r) {
                    rt.notifyReadOnly(entity: entityId)
                    return
                }
            }
            for (id, w) in wins { pending[id] = w }
            taps += 1
            Task {
                // 順に保存: 下げる方(元の勝者)が先。途中で失敗したら残りは送らない(勝者が 2 人になる途中の形を残さない)
                var failed = false
                for (id, data) in plan {
                    if !failed {
                        failed = !(await rt.updateRecord(entity: entityId, recordId: id, data: data))
                        if failed { print("[Metamon] bracket not saved: \(entityId)/\(id)") }
                    }
                    // 失敗なら保存待ちを外す = ストアの値に戻る
                    if let w = wins[id], pending[id] == w { pending[id] = nil }
                }
            }
        }
    }

    // MARK: - 計算(描画の木 RenderTree と共用)

    /// 保存の順: 勝ち数の小さい方(取り消し・元の勝者を戻す方)から。同じなら id 順。
    static func writeOrder(_ wins: [String: Int]) -> [(String, Int)] {
        wins.sorted { $0.value != $1.value ? $0.value < $1.value : $0.key < $1.key }.map { ($0.key, $0.value) }
    }

    /// 保存する順と中身(recordId → data)。開始時刻を書く時は先に書いてから勝ち数(勝ちが残れば必ず開始時刻がある)、
    /// 消す時は勝ち数を戻してから消す。勝ち数は `writeOrder`。同じ人の開始時刻は勝ち数と一緒に送る。
    static func writePlan(wins: [String: Int], starts: [String: JSONValue], winsField: String, startedField: String?) -> [(String, [String: JSONValue])] {
        let winWrites: [(String, [String: JSONValue])] = writeOrder(wins).map { id, w in
            var d: [String: JSONValue] = [winsField: .number(Double(w))]
            if let f = startedField, let v = starts[id] { d[f] = v }
            return (id, d)
        }
        guard let f = startedField, !starts.isEmpty else { return winWrites }
        let only = starts.keys.filter { wins[$0] == nil }.sorted().map { ($0, [f: starts[$0]!]) }
        let setting = starts.values.contains { !$0.isNull }
        return setting ? only + winWrites : winWrites + only
    }

    /// 開始時刻を保存できる field(datetime・computed でない)。無ければ nil(時刻の規則で固定)。
    static func startedField(_ c: Component, in entity: Entity?) -> String? {
        guard let id = c.startedField, let f = entity?.field(id), f.computed == nil, f.type == .datetime else { return nil }
        return id
    }

    /// VoiceOver の試合の見出し: 「準決勝 第 1 試合」。
    static func matchLabel(_ m: BracketDraw.Match, roundTitle: String) -> String {
        L("\(roundTitle) 第 \(m.index + 1) 試合")
    }

    /// 勝ち数を保存できる field(integer・computed でない)。条件を満たさなければ nil(表示だけ)。
    static func winsField(_ c: Component, in entity: Entity?) -> String? {
        guard let id = c.winsField, let f = entity?.field(id), f.computed == nil, f.type == .integer else { return nil }
        return id
    }

    static func draw(_ c: Component, rt: any DSLRuntime, pending: [String: Int]) -> BracketDraw {
        BracketDraw(records: rt.records(for: c.entity ?? ""), winsField: c.winsField ?? "", seedField: c.seedField,
                    startedField: startedField(c, in: rt.dsl.entity(c.entity ?? "")), pending: pending)
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
