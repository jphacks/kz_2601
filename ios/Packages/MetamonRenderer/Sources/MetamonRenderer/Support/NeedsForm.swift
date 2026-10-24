import SwiftUI
import MetamonDSL

/// フォーム外(screen.entity 無し)に入力が置かれた時のプレースホルダ。
struct NeedsForm: View {
    let c: Component
    var body: some View { UnsupportedComponent(label: "\(c.rawType) は screen.entity が必要です") }
}

/// 入力の上にキャプションを付ける共通ラッパ(小・secondary。フォーカス中はブランド色)。
struct LabeledInput<Content: View>: View {
    var label: String?
    var focused = false
    @ViewBuilder var content: Content

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            if let label, !label.isEmpty {
                Text(label)
                    .font(RenderTheme.Fonts.inputLabel)
                    .foregroundStyle(focused ? AnyShapeStyle(RenderTheme.Colors.brandA) : AnyShapeStyle(.secondary))
                    .padding(.leading, 2)
                    .animation(RenderTheme.Motion.select, value: focused)
            }
            content
        }
    }
}

/// コントロール型の入力(datePicker / timePicker / colorPicker / toggle / stepper)の 1 行。
/// 入力と同じ器の中に「アイコン + ラベル … コントロール」。ラベルはコントロール側が持つので上キャプションは出さない。
struct InputRow<Control: View>: View {
    let icon: String
    let label: String
    @ViewBuilder var control: Control

    var body: some View {
        HStack(spacing: RenderTheme.Spacing.sm + 2) {
            InputIcon(name: icon)
            Text(label)
                .font(RenderTheme.Fonts.inputValue)
                .lineLimit(1)
                .minimumScaleFactor(0.7)
                .layoutPriority(1)
            Spacer(minLength: RenderTheme.Spacing.xs)
            control
                .layoutPriority(2)
        }
        .renderInputChrome()
    }
}
