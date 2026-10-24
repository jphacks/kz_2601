import SwiftUI
import MetamonDSL

/// `list` の1行(`ListItem` テンプレート: title / subtitle / trailing / icon / image)。
/// 行自身は背景を持たない(ListLeaf が 1 枚のカードにまとめ、行間を hairline で区切る)。
/// - 先頭(#89): 画像サムネ(角丸 10)> アイコン(ブランドの薄い角丸)> カテゴリ色ドット(enum field の値から安定割当)。
/// - VoiceOver: 行は 1 要素。「title, subtitle, trailing」の順に読む(icon / chevron / 画像は装飾なので隠す)。
/// - Dynamic Type(アクセシビリティサイズ): 横並びだと title が潰れるので trailing を下段に落とし、title は 2 行まで。
struct ListRow: View {
    let item: ListItem?
    let record: Record
    let entity: Entity?
    var showsChevron = false
    @Environment(\.dynamicTypeSize) private var typeSize

    private func text(_ fieldId: String?) -> String? {
        guard let fieldId else { return nil }
        return FieldCoding.display(record[fieldId], field: entity?.field(fieldId))
    }

    /// trailing が数値/通貨 field なら等幅・太めで右寄せ(金額が縦に揃う)。
    private var trailingIsNumeric: Bool {
        item?.trailing.flatMap { entity?.field($0)?.type.isNumeric } ?? false
    }

    private var titleText: String { text(item?.title ?? entity?.titleField) ?? "" }
    private var subtitleText: String? { text(item?.subtitle).flatMap { $0.isEmpty ? nil : $0 } }
    private var trailingText: String? { text(item?.trailing).flatMap { $0.isEmpty ? nil : $0 } }

    /// subtitle / title のどちらかが enum(カテゴリ)なら、その値から色を決める。
    private var categoryColor: Color? {
        for fieldId in [item?.subtitle, item?.title].compactMap({ $0 }) {
            if entity?.field(fieldId)?.type == .enum, let v = text(fieldId), !v.isEmpty {
                return RenderTheme.Colors.category(v)
            }
        }
        return nil
    }

    private var accessibilityText: String {
        [titleText, subtitleText, trailingText].compactMap { $0 }.filter { !$0.isEmpty }.joined(separator: ", ")
    }

    var body: some View {
        let stacked = typeSize.isAccessibilitySize
        let layout = stacked ? AnyLayout(VStackLayout(alignment: .leading, spacing: RenderTheme.Spacing.xs)) : AnyLayout(HStackLayout(spacing: RenderTheme.Spacing.row))
        HStack(alignment: stacked ? .top : .center, spacing: RenderTheme.Spacing.row) {
            if let img = item?.image {
                PhotoView(value: record[img], height: 52).frame(width: 52)
            } else if let icon = item?.icon {
                Image(systemName: icon)
                    .font(.body.weight(.semibold))
                    .foregroundStyle(RenderTheme.Colors.brandA)
                    .frame(width: 36, height: 36)
                    .background(RenderTheme.Colors.brandSoft, in: RoundedRectangle(cornerRadius: RenderTheme.Radius.inner, style: .continuous))
                    .accessibilityHidden(true)
            } else if let dot = categoryColor {
                Circle()
                    .fill(dot)
                    .frame(width: 10, height: 10)
                    .overlay(Circle().strokeBorder(.white.opacity(0.6), lineWidth: 1))
                    .padding(.top, stacked ? 6 : 0)
                    .accessibilityHidden(true)
            }
            layout {
                VStack(alignment: .leading, spacing: 3) {
                    Text(titleText)
                        .font(RenderTheme.Fonts.rowTitle)
                        .lineLimit(stacked ? 2 : 1)
                    if let sub = subtitleText {
                        Text(sub).font(RenderTheme.Fonts.rowSubtitle).foregroundStyle(.secondary).lineLimit(stacked ? 2 : 1)
                    }
                }
                if !stacked { Spacer(minLength: RenderTheme.Spacing.sm) }
                if let trailing = trailingText {
                    Text(trailing)
                        .font(trailingIsNumeric ? RenderTheme.Fonts.rowTrailing : .subheadline.monospacedDigit())
                        .foregroundStyle(trailingIsNumeric ? .primary : .secondary)
                        .lineLimit(1)
                        .minimumScaleFactor(0.7)
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            if showsChevron {
                Image(systemName: "chevron.right")
                    .font(.caption.weight(.semibold))
                    .foregroundStyle(.tertiary)
                    .frame(maxHeight: .infinity)
                    .accessibilityHidden(true)
            }
        }
        .padding(.horizontal, RenderTheme.Spacing.md)
        .padding(.vertical, RenderTheme.Spacing.row)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(accessibilityText)
    }
}
