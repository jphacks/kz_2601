import SwiftUI
import MetamonDSL

/// `picker`: enum field の options から選択。style(menu|segmented|wheel、省略時は 4 件以下なら segmented)。
/// 見た目: segmented はチップ型(選択中はブランドグラデーション + 白文字)、menu は入力と同じ器 + chevron、wheel はそのまま。
struct PickerLeaf: View {
    let c: Component
    let ctx: RenderContext
    @FocusState private var focused: Bool

    var body: some View {
        if let form = ctx.form, let field = c.field {
            let fieldDef = form.entity.field(field)
            let selection = form.string(field)
            let options = Self.options(base: fieldDef?.options ?? [], current: selection.wrappedValue)
            let picker = Picker(c.label ?? "", selection: selection) {
                ForEach(options, id: \.self) { Text($0).tag($0) }
            }
            #if os(macOS)
            // Mac: 4 件以下はラジオ・それ以上はメニュー(DSL の style が優先・#942)
            LabeledInput(label: c.label) {
                Group {
                    switch MacPickerStyle.choose(hint: c.style, optionCount: options.count) {
                    case .radioGroup: picker.pickerStyle(.radioGroup).labelsHidden()
                    case .segmented: picker.pickerStyle(.segmented).labelsHidden()
                    case .menu: picker.pickerStyle(.menu).labelsHidden()
                    }
                }
                .focused($focused) // フルキーボードアクセスが on の時の Tab の止まり先(#2065)
                .macFormTabStop(c, $focused)
            }
            #else
            switch Self.style(c, optionCount: options.count) {
            case "segmented":
                LabeledInput(label: c.label) {
                    ChipSegments(options: options, selection: selection)
                        .accessibilityElement(children: .contain)
                        .accessibilityLabel(c.label ?? "")
                }
            case "wheel":
                picker.wheelPickerStyle().frame(height: 120)
            default:
                // menu は List/Form の外ではラベルを描かないので、他の入力と同じく上にラベルを出す
                LabeledInput(label: c.label) {
                    MenuSelect(icon: FieldIcon.infer(component: c, field: fieldDef), label: c.label ?? "",
                               selection: selection, options: options.map { ($0, $0) })
                }
            }
            #endif
        } else {
            NeedsForm(c: c)
        }
    }

    /// 選択肢から消した値を持つ既存の記録は、その値も並べて表示を保つ(#194)。適合テストの描画の木も同じ並びを使う。
    static func options(base: [String], current: String) -> [String] {
        current.isEmpty || base.contains(current) ? base : base + [current]
    }

    /// 省略時は 4 件以下なら segmented。
    static func style(_ c: Component, optionCount: Int) -> String {
        c.style ?? (optionCount <= 4 ? "segmented" : "menu")
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
