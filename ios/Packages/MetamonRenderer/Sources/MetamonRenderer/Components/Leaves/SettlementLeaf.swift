import SwiftUI
import MetamonDSL

/// `settlement`(#101): balanceField(正 = 受け取る / 負 = 払う。多くは computed)から「A → B ¥1,200」を貪欲法で列挙。
/// 全員 0 なら「精算は不要です」。見た目は ListLeaf と同じ 1 枚カード + hairline 区切り。
struct SettlementLeaf: View {
    @Environment(\.dslRuntime) private var rt
    let c: Component

    var body: some View {
        let entityId = c.entity ?? ""
        let entity = rt.dsl.entity(entityId)
        let nameField = c.nameField ?? entity?.displayTitleField ?? "name"
        let records = rt.viewRecords(for: entityId)
        let balances: [(name: String, balance: Double)] = records.compactMap { r in
            guard let b = r[c.balanceField ?? "balance"].doubleValue else { return nil }
            return (rt.display(r[nameField], field: entity?.field(nameField)), b)
        }
        let transfers = Settlement.transfers(balances)
        VStack(alignment: .leading, spacing: 0) {
            HStack(spacing: RenderTheme.Spacing.sm) {
                Image(systemName: "arrow.left.arrow.right.circle.fill").foregroundStyle(.tint).accessibilityHidden(true)
                Text(c.title ?? "精算").font(RenderTheme.Fonts.cardTitle)
            }
            .padding(.horizontal, RenderTheme.Spacing.md)
            .padding(.vertical, RenderTheme.Spacing.row)
            .accessibilityAddTraits(.isHeader)
            Hairline()
            if records.isEmpty {
                EmptyPlaceholder(icon: "person.3", emptyText: "メンバーがいません", state: rt.emptyState(for: entityId)) {
                    Task { await rt.load(entities: [entityId], force: true) }
                }
                .padding(RenderTheme.Spacing.md)
            } else if transfers.isEmpty {
                Label("精算は不要です", systemImage: "checkmark.circle")
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
                    .padding(RenderTheme.Spacing.md)
            } else {
                ForEach(Array(transfers.enumerated()), id: \.offset) { index, t in
                    if index > 0 { Hairline(inset: RenderTheme.Spacing.md) }
                    TransferRow(transfer: t)
                }
            }
        }
        .frame(maxHeight: .infinity, alignment: .top)
        .renderCard(padding: 0)
    }
}

private struct TransferRow: View {
    let transfer: Settlement.Transfer

    var body: some View {
        HStack(spacing: RenderTheme.Spacing.sm) {
            Text(transfer.from).font(RenderTheme.Fonts.rowTitle).lineLimit(1)
            Image(systemName: "arrow.right").font(.caption.weight(.bold)).foregroundStyle(.secondary)
            Text(transfer.to).font(RenderTheme.Fonts.rowTitle).lineLimit(1)
            Spacer(minLength: RenderTheme.Spacing.sm)
            Text(FieldCoding.currency(transfer.amount))
                .font(RenderTheme.Fonts.rowTrailing)
                .lineLimit(1)
                .fixedSize()
                .layoutPriority(1)
        }
        .padding(.horizontal, RenderTheme.Spacing.md)
        .padding(.vertical, RenderTheme.Spacing.row)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("\(transfer.from) から \(transfer.to) へ \(FieldCoding.currency(transfer.amount))")
    }
}
