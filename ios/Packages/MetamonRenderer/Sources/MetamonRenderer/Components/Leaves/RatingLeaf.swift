import SwiftUI
import MetamonDSL

/// `rating`: 星 1〜max(既定 5)のタップ評価。同じ星をもう一度タップすると 0(未評価)に戻る。
/// 見た目: 上ラベル + 器の中にブランドグラデーションの塗り星(未選択は tertiary の輪郭)。タップで 0.97→1 のスプリング。
struct RatingLeaf: View {
    let c: Component
    let ctx: RenderContext

    var body: some View {
        if let form = ctx.form, let field = c.field {
            let count = NumericInput.ratingCount(c.max)
            let value = form.double(field, fallback: 0)
            let current = NumericInput.ratingValue(value.wrappedValue, count: count)
            LabeledInput(label: c.label) {
                HStack(spacing: RenderTheme.Spacing.sm) {
                    ForEach(1...count, id: \.self) { i in
                        Button {
                            withAnimation(RenderTheme.Motion.press) {
                                value.wrappedValue = (i == current) ? 0 : Double(i)
                            }
                        } label: {
                            Image(systemName: i <= current ? "star.fill" : "star")
                                .font(.title2.weight(.medium))
                                .foregroundStyle(i <= current ? AnyShapeStyle(RenderTheme.brandGradient) : AnyShapeStyle(.tertiary))
                                .scaleEffect(i == current ? 1.1 : 1)
                                .frame(minWidth: 32, minHeight: 32)
                                .contentShape(Rectangle())
                        }
                        .buttonStyle(.plain)
                    }
                    Spacer(minLength: 0)
                }
                // Mac: ←→・数字キー・VoiceOver の増減で付ける(#941)
                .iosRatingAccessibility(label: c.label, current: current, count: count) { n in
                    withAnimation(RenderTheme.Motion.press) { value.wrappedValue = Double(n) }
                }
                .macRatingControl(label: c.label, current: current, count: count) { n in
                    withAnimation(RenderTheme.Motion.press) { value.wrappedValue = Double(n) }
                }
                .renderInputChrome()
            }
        } else {
            NeedsForm(c: c)
        }
    }
}

/// rating の VoiceOver 用の文字と増減(iOS)。星ごとのボタンでなく 1 つの調整できる要素として読む。
enum RatingAccessibility {
    /// 値の読み上げ「星 3、全部で 5」
    static func value(current: Int, count: Int) -> String {
        L("星 \(current)、全部で \(count)")
    }

    /// 部品のラベル(空なら「評価」)
    static func label(_ raw: String?) -> String {
        raw.flatMap { $0.isEmpty ? nil : $0 } ?? L("評価")
    }
}

extension View {
    /// iOS: 星の並びを 1 要素にして、ラベル + 値 + 上下スワイプの増減にする。Mac は `macRatingControl` が受け持つ。
    @ViewBuilder
    func iosRatingAccessibility(label: String?, current: Int, count: Int, set: @escaping (Int) -> Void) -> some View {
        #if os(macOS)
        self
        #else
        accessibilityElement(children: .ignore)
            .accessibilityLabel(RatingAccessibility.label(label))
            .accessibilityValue(RatingAccessibility.value(current: current, count: count))
            .accessibilityAdjustableAction { direction in
                switch direction {
                case .increment: set(MacRatingKeys.adjusted(increment: true, current: current, count: count))
                case .decrement: set(MacRatingKeys.adjusted(increment: false, current: current, count: count))
                @unknown default: break
                }
            }
        #endif
    }
}
