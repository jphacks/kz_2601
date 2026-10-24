import SwiftUI
import MetamonDSL

/// `list` の1行(`ListItem` テンプレート: title / subtitle / trailing / icon / image)。
/// 行自身は背景を持たない(ListLeaf が 1 枚のカードにまとめ、行間を hairline で区切る)。
struct ListRow: View {
    let item: ListItem?
    let record: Record
    let entity: Entity?
    var showsChevron = false

    private func text(_ fieldId: String?) -> String? {
        guard let fieldId else { return nil }
        return FieldCoding.display(record[fieldId], field: entity?.field(fieldId))
    }

    /// trailing が数値/通貨 field なら等幅・太めで右寄せ(金額が縦に揃う)。
    private var trailingIsNumeric: Bool {
        item?.trailing.flatMap { entity?.field($0)?.type.isNumeric } ?? false
    }

    var body: some View {
        HStack(spacing: RenderTheme.Spacing.row) {
            if let img = item?.image {
                PhotoView(value: record[img], height: 52).frame(width: 52)
            } else if let icon = item?.icon {
                Image(systemName: icon)
                    .font(.body.weight(.medium))
                    .foregroundStyle(.tint)
                    .frame(width: 32, height: 32)
                    .background(Color.accentColor.opacity(0.12), in: RoundedRectangle(cornerRadius: RenderTheme.Radius.control, style: .continuous))
            }
            VStack(alignment: .leading, spacing: 2) {
                Text(text(item?.title ?? entity?.titleField) ?? "")
                    .font(RenderTheme.Fonts.rowTitle)
                    .lineLimit(1)
                if let sub = text(item?.subtitle), !sub.isEmpty {
                    Text(sub).font(RenderTheme.Fonts.rowSubtitle).foregroundStyle(.secondary).lineLimit(1)
                }
            }
            Spacer(minLength: RenderTheme.Spacing.sm)
            if let trailing = text(item?.trailing), !trailing.isEmpty {
                Text(trailing)
                    .font(trailingIsNumeric ? RenderTheme.Fonts.rowTrailing : .subheadline.monospacedDigit())
                    .foregroundStyle(trailingIsNumeric ? .primary : .secondary)
                    .lineLimit(1)
            }
            if showsChevron {
                Image(systemName: "chevron.right")
                    .font(.caption.weight(.semibold))
                    .foregroundStyle(.tertiary)
            }
        }
        .padding(.horizontal, RenderTheme.Spacing.md)
        .padding(.vertical, RenderTheme.Spacing.row)
    }
}
