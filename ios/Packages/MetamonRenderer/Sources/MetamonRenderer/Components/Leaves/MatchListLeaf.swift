// 審査用ビルド(APP_STORE_REVIEW)は共有を外しているので、仲間探し(#507)も外す。
#if !APP_STORE_REVIEW
import SwiftUI
import MetamonDSL

/// `matchList`(#507): 届いた「つながりたい」(承認 / 見送る)・送った「つながりたい」(取り下げる)・つながった人(長押しで解除)。
/// 行は相手のプロフィールのレコード(`profileRecordId`)を `ListRow` で描く。どの行も長押しで通報・ブロック(`ModerationMenuItems`)。
/// 操作はホスト(`MatchingHost`)が楽観的に反映し、失敗したら戻して理由を出す。
struct MatchListLeaf: View {
    @Environment(\.dslRuntime) private var rt
    let c: Component
    let ctx: RenderContext
    @State private var unmatching: MatchInterest?
    @State private var taps = 0

    var body: some View {
        if let box = MatchBox(show: c.show), let entityId = c.entity, let entity = rt.dsl.entity(entityId), entity.isMatchProfile {
            VStack(alignment: .leading, spacing: RenderTheme.Spacing.sm) {
                if let title = c.title, !title.isEmpty {
                    Text(title).font(RenderTheme.Fonts.cardTitle).accessibilityAddTraits(.isHeader)
                }
                rows(box: box, entity: entity)
            }
            .task(id: "\(entityId)/\(box.rawValue)") { await rt.matching?.refreshMatches([box]) }
            .sensoryFeedback(.selection, trigger: taps)
            .alert(Text(L("つながりを解除しますか?")), isPresented: Binding(get: { unmatching != nil }, set: { if !$0 { unmatching = nil } }),
                   presenting: unmatching) { item in
                Button(L("解除する"), role: .destructive) { Task { await rt.matching?.unmatch(item) } }
                Button(L("キャンセル"), role: .cancel) {}
            } message: { _ in
                Text(L("お互いに連絡先が見えなくなります。相手に通知は送られません。"))
            }
        } else {
            UnsupportedComponent(label: "matchList")
        }
    }

    @ViewBuilder
    private func rows(box: MatchBox, entity: Entity) -> some View {
        let items = rt.matching?.matchItems(box)
        let profiles = Dictionary(rt.records(for: entity.id).map { ($0.id, $0) }, uniquingKeysWith: { a, _ in a })
        if let items, !items.isEmpty {
            LazyVStack(spacing: 0) {
                ForEach(Array(items.enumerated()), id: \.element.uid) { index, item in
                    if index > 0 { Hairline(inset: RenderTheme.Spacing.md) }
                    row(item, box: box, entity: entity, profile: item.profileRecordId.flatMap { profiles[$0] })
                }
            }
            .frame(maxHeight: .infinity, alignment: .top)
            .renderCard(padding: 0)
        } else {
            EmptyPlaceholder(icon: Self.icon(box), emptyText: c.emptyText ?? Self.emptyText(box), state: emptyState(items: items, box: box)) {
                Task { await rt.matching?.refreshMatches([box]) }
            }
        }
    }

    private func emptyState(items: [MatchInterest]?, box: MatchBox) -> EmptyState {
        guard let host = rt.matching else { return .empty } // モック・プレビュー: 誰もいない
        if items != nil { return .empty }
        return host.matchLoadFailed(box) ? .unavailable : .loading
    }

    @ViewBuilder
    private func row(_ item: MatchInterest, box: MatchBox, entity: Entity, profile: Record?) -> some View {
        Group {
            if let profile {
                ListRow(item: c.item, record: profile, entity: entity, showsChevron: c.onItemTap != nil,
                        accessory: accessory(item, box: box))
                    .contentShape(Rectangle())
                    .onTapGesture { open(profile) }
                    .macHelp(c.onItemTap != nil ? L("クリックで開きます") : nil)
                    .accessibilityAddTraits(c.onItemTap != nil ? .isButton : [])
                    .accessibilityAction { open(profile) }
                    .accessibilityActions { ModerationAccessibilityActions(rt: rt, entity: entity.id, record: profile) }
            } else {
                missingProfileRow(item, box: box)
            }
        }
        .contextMenu {
            ComponentTalkMenuItem(component: c, ctx: ctx) // 行のメニューが先に出るので、ここにも置く(#1323)
            if box == .matches {
                Button(L("つながりを解除"), systemImage: "person.crop.circle.badge.xmark", role: .destructive) { unmatching = item }
            }
            if let profile { ModerationMenuItems(entity: entity.id, record: profile) }
        }
    }

    /// プロフィールが消された・まだ読めていない人。操作だけ出す。
    private func missingProfileRow(_ item: MatchInterest, box: MatchBox) -> some View {
        HStack(spacing: RenderTheme.Spacing.row) {
            Text(L("プロフィールが見つかりません"))
                .font(RenderTheme.Fonts.rowSubtitle)
                .foregroundStyle(.secondary)
                .frame(maxWidth: .infinity, alignment: .leading)
            if let a = accessory(item, box: box) { a.view }
        }
        .padding(.horizontal, RenderTheme.Spacing.md)
        .padding(.vertical, RenderTheme.Spacing.row)
    }

    /// 行の右端: incoming = 承認・見送る / outgoing = 取り下げる / matches = なし(長押しで解除)。
    private func accessory(_ item: MatchInterest, box: MatchBox) -> RowAccessory? {
        guard let host = rt.matching else { return nil }
        switch box {
        case .incoming:
            let accept = { taps += 1; Task { await host.acceptInterest(item) } }
            let decline = { taps += 1; Task { await host.declineInterest(item, box: .incoming) } }
            return RowAccessory(view: AnyView(HStack(spacing: RenderTheme.Spacing.xs) {
                Button(L("見送る"), action: decline)
                    .buttonStyle(.bordered)
                    .accessibilityIdentifier("matchList.decline")
                Button(L("承認"), action: accept)
                    .buttonStyle(.borderedProminent)
                    .tint(RenderTheme.Colors.brandA)
                    .accessibilityIdentifier("matchList.accept")
            }
            .buttonBorderShape(.capsule)
            .controlSize(.small)
            .font(.subheadline.weight(.semibold))),
                                actions: [.init(label: L("承認"), perform: { accept() }), .init(label: L("見送る"), perform: { decline() })])
        case .outgoing:
            let withdraw = { taps += 1; Task { await host.declineInterest(item, box: .outgoing) } }
            return RowAccessory(view: AnyView(Button(L("取り下げる"), action: withdraw)
                .buttonStyle(.bordered)
                .buttonBorderShape(.capsule)
                .controlSize(.small)
                .font(.subheadline.weight(.semibold))
                .accessibilityIdentifier("matchList.withdraw")),
                                actions: [.init(label: L("取り下げる"), perform: { withdraw() })])
        case .matches:
            return RowAccessory(view: AnyView(EmptyView()),
                                actions: [.init(label: L("つながりを解除"), perform: { unmatching = item })])
        }
    }

    private func open(_ profile: Record) {
        guard let action = c.onItemTap else { return }
        Task { await rt.run(action, ctx.actionContext(recordId: profile.id)) }
    }

    static func icon(_ box: MatchBox) -> String {
        switch box {
        case .incoming: "tray.and.arrow.down"
        case .outgoing: "paperplane"
        case .matches: "person.2"
        }
    }

    static func emptyText(_ box: MatchBox) -> String {
        switch box {
        case .incoming: L("まだ届いていません")
        case .outgoing: L("送った「つながりたい」はありません")
        case .matches: L("まだ誰ともつながっていません")
        }
    }
}
#endif
