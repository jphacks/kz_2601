import SwiftUI
import MetamonDSL

/// `picker`: enum field の options から選択。style(menu|segmented|wheel、省略時は 4 件以下なら segmented)。
/// 見た目: segmented はチップ型(選択中はブランドグラデーション + 白文字)、menu は入力と同じ器 + chevron、wheel はそのまま。
struct PickerLeaf: View {
    let c: Component
    let ctx: RenderContext

    var body: some View {
        if let form = ctx.form, let field = c.field {
            let fieldDef = form.entity.field(field)
            let options = fieldDef?.options ?? []
            let selection = form.string(field)
            let picker = Picker(c.label ?? "", selection: selection) {
                ForEach(options, id: \.self) { Text($0).tag($0) }
            }
            switch c.style ?? (options.count <= 4 ? "segmented" : "menu") {
            case "segmented":
                LabeledInput(label: c.label) {
                    ChipSegments(options: options, selection: selection)
                        .accessibilityElement(children: .contain)
                        .accessibilityLabel(c.label ?? "")
                }
            case "wheel":
                picker.pickerStyle(.wheel).frame(height: 120)
            default:
                // menu は List/Form の外ではラベルを描かないので、他の入力と同じく上にラベルを出す
                LabeledInput(label: c.label) {
                    HStack(spacing: RenderTheme.Spacing.sm + 2) {
                        InputIcon(name: FieldIcon.infer(component: c, field: fieldDef))
                        picker
                            .pickerStyle(.menu)
                            .labelsHidden()
                            .tint(.primary)
                            .font(RenderTheme.Fonts.inputValue)
                        Spacer(minLength: 0)
                        Image(systemName: "chevron.up.chevron.down")
                            .font(.caption.weight(.semibold))
                            .foregroundStyle(RenderTheme.Colors.brandA)
                            .accessibilityHidden(true)
                    }
                    .renderInputChrome()
                }
            }
        } else {
            NeedsForm(c: c)
        }
    }
}

/// チップ型セグメント。選択中はブランドグラデーションのピルが `matchedGeometryEffect` で滑る。
/// 文字が長い / Dynamic Type 大で 1 行に収まらない時は横スクロール。
struct ChipSegments: View {
    let options: [String]
    @Binding var selection: String
    @Namespace private var ns
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    var body: some View {
        ViewThatFits(in: .horizontal) {
            chips(fill: true)
            ScrollView(.horizontal, showsIndicators: false) { chips(fill: false) }
        }
    }

    private func chips(fill: Bool) -> some View {
        HStack(spacing: 6) {
            ForEach(options, id: \.self) { option in
                let selected = option == selection
                Button {
                    withAnimation(reduceMotion ? nil : RenderTheme.Motion.select) { selection = option }
                } label: {
                    Text(option)
                        .font(RenderTheme.Fonts.chip)
                        .lineLimit(1)
                        .minimumScaleFactor(0.8)
                        .padding(.horizontal, RenderTheme.Spacing.sm + 4)
                        .padding(.vertical, RenderTheme.Spacing.sm + 2)
                        .frame(maxWidth: fill ? .infinity : nil, minHeight: 40)
                        .foregroundStyle(selected ? Color.white : Color.primary)
                        .background {
                            if selected {
                                Capsule().fill(RenderTheme.brandFill)
                                    .shadow(color: RenderTheme.Shadow.brandColor, radius: 6, y: 3)
                                    .matchedGeometryEffect(id: "chip", in: ns)
                            }
                        }
                        .contentShape(Capsule())
                }
                .buttonStyle(ChipPressStyle())
                .accessibilityAddTraits(selected ? [.isSelected] : [])
            }
        }
        .padding(4)
        .background(RenderTheme.Colors.surfaceElevated, in: Capsule())
        .overlay(Capsule().strokeBorder(RenderTheme.Colors.hairline))
    }
}

private struct ChipPressStyle: ButtonStyle {
    func makeBody(configuration: Configuration) -> some View {
        configuration.label.pressable(configuration.isPressed)
    }
}
