import SwiftUI
import MetamonDSL

/// `list` の1行(`ListItem` テンプレート: title / subtitle / trailing / icon / image)。
/// 行自身は背景を持たない(ListLeaf が 1 枚のカードにまとめ、行間を hairline で区切る)。
/// - VoiceOver: 行は 1 要素。「title, subtitle, trailing」の順に読む(icon / chevron / 画像は装飾なので隠す)。
/// - Dynamic Type(アクセシビリティサイズ): 横並びだと title が潰れるので trailing を下段に落とし、title は 2 行まで。
struct ListRow: View {
    let item: ListItem?
    let record: Record
    let entity: Entity?
    var showsChevron = false
    @Environment(\.dynamicTypeSize) private var typeSize
    @Environment(\.dslRuntime) private var rt

    private func text(_ fieldId: String?) -> String? {
        guard let fieldId else { return nil }
        return rt.display(record[fieldId], field: entity?.field(fieldId))
    }

    /// trailing が数値/通貨 field なら等幅・太めで右寄せ(金額が縦に揃う)。
    private var trailingIsNumeric: Bool {
        item?.trailing.flatMap { entity?.field($0)?.type.isNumeric } ?? false
    }

    private var titleText: String { text(item?.title ?? entity?.displayTitleField) ?? "" }
    private var subtitleText: String? { text(item?.subtitle).flatMap { $0.isEmpty ? nil : $0 } }
    private var trailingText: String? { text(item?.trailing).flatMap { $0.isEmpty ? nil : $0 } }

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
                    .font(.body.weight(.medium))
                    .foregroundStyle(.tint)
                    .frame(width: 32, height: 32)
                    .background(Color.accentColor.opacity(0.12), in: RoundedRectangle(cornerRadius: RenderTheme.Radius.control, style: .continuous))
                    .accessibilityHidden(true)
            }
            layout {
                VStack(alignment: .leading, spacing: 2) {
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
