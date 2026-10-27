import SwiftUI
import MetamonDSL

/// `list` の1行(`ListItem` テンプレート: title / subtitle / trailing / icon / image)。
/// 行自身は背景を持たない(ListLeaf が 1 枚のカードにまとめ、行間を hairline で区切る)。
/// - 先頭(#89): 画像サムネ(角丸 10)> アイコン(ブランドの薄い角丸)> カテゴリ色ドット(enum field の値から安定割当)。
/// - VoiceOver: 行は 1 要素。「title, subtitle, trailing」の順に読む(icon / chevron / 画像は装飾なので隠す)。
/// - Dynamic Type(アクセシビリティサイズ): 横並びだと title が潰れるので trailing を下段に落とし、title は 2 行まで。
/// - `toggleOn` が非 nil(#108 `list.item.toggle`)なら行頭にチェック丸。チェック済みは title に打ち消し線 + 文字を薄く。
struct ListRow: View {
    let item: ListItem?
    let record: Record
    let entity: Entity?
    var showsChevron = false
    var toggleOn: Bool? = nil
    var onToggle: (() -> Void)? = nil
    /// false なら丸を薄く描く(他人の共有レコード・#249)。押すと onToggle が理由を出す
    var toggleEnabled = true
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

    private var checked: Bool { toggleOn == true }

    /// 行頭のチェック丸。行タップ(フォームを開く)とは別のボタン。44pt の当たり判定。
    private func checkCircle(_ on: Bool) -> some View {
        Button {
            onToggle?()
        } label: {
            Image(systemName: on ? "checkmark.circle.fill" : "circle")
                .font(.title2)
                .foregroundStyle(on ? RenderTheme.Colors.brandA : Color.secondary)
                .opacity(toggleEnabled ? 1 : 0.4)
                .contentTransition(.symbolEffect(.replace))
                .frame(width: 28, height: 28)
                .padding(8)
                .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .padding(-8)
        .accessibilityHidden(true) // 行 1 要素の accessibilityAction で操作する
    }

    var body: some View {
        let stacked = typeSize.isAccessibilitySize
        let layout = stacked ? AnyLayout(VStackLayout(alignment: .leading, spacing: RenderTheme.Spacing.xs)) : AnyLayout(HStackLayout(spacing: RenderTheme.Spacing.row))
        HStack(alignment: stacked ? .top : .center, spacing: RenderTheme.Spacing.row) {
            if let on = toggleOn {
                checkCircle(on)
            }
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
                        .strikethrough(checked)
                        .foregroundStyle(checked ? .secondary : .primary)
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
            .opacity(checked ? 0.6 : 1)
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
        .accessibilityValue(toggleOn.map { $0 ? "チェック済み" : "未チェック" } ?? "")
        .accessibilityActions {
            if let onToggle {
                Button(checked ? "チェックを外す" : "チェックする", action: onToggle)
            }
        }
    }
}
