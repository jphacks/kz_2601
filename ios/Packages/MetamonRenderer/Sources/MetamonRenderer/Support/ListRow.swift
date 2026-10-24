import SwiftUI
import MetamonDSL

/// `list` の1行(`ListItem` テンプレート: title / subtitle / trailing / icon / image)。
/// 行自身は背景を持たない(ListLeaf が 1 枚のカードにまとめ、行間を hairline で区切る)。
/// 先頭: 画像サムネ(角丸 10)> アイコン(ブランドの薄い角丸)> カテゴリ色ドット(enum field の値から安定割当)。
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

    /// subtitle / title のどちらかが enum(カテゴリ)なら、その値から色を決める。
    private var categoryColor: Color? {
        for fieldId in [item?.subtitle, item?.title].compactMap({ $0 }) {
            if entity?.field(fieldId)?.type == .enum, let v = text(fieldId), !v.isEmpty {
                return RenderTheme.Colors.category(v)
            }
        }
        return nil
    }

    var body: some View {
        HStack(spacing: RenderTheme.Spacing.row) {
            if let img = item?.image {
                PhotoView(value: record[img], height: 52).frame(width: 52)
            } else if let icon = item?.icon {
                Image(systemName: icon)
                    .font(.body.weight(.semibold))
                    .foregroundStyle(RenderTheme.Colors.brandA)
                    .frame(width: 36, height: 36)
                    .background(RenderTheme.Colors.brandSoft, in: RoundedRectangle(cornerRadius: RenderTheme.Radius.inner, style: .continuous))
            } else if let dot = categoryColor {
                Circle()
                    .fill(dot)
                    .frame(width: 10, height: 10)
                    .overlay(Circle().strokeBorder(.white.opacity(0.6), lineWidth: 1))
                    .accessibilityHidden(true)
            }
            VStack(alignment: .leading, spacing: 3) {
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
