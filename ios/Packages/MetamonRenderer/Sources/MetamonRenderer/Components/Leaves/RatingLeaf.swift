import SwiftUI
import MetamonDSL

/// `rating`: 星 1〜max(既定 5)のタップ評価。同じ星をもう一度タップすると 0(未評価)に戻る。
/// 見た目: 上ラベル + 器の中にブランドグラデーションの塗り星(未選択は tertiary の輪郭)。タップで 0.97→1 のスプリング。
/// Mac では Tab で止まり、← → で上げ下げ、0〜9 の数字キーで直に付けられる。VoiceOver は 1 つの増減できる要素として読む(#941)。
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
                            set(i == current ? 0 : i, on: value)
                        } label: {
                            Image(systemName: i <= current ? "star.fill" : "star")
                                .font(.title2.weight(.medium))
                                .foregroundStyle(i <= current ? AnyShapeStyle(RenderTheme.brandGradient) : AnyShapeStyle(.tertiary))
                                .scaleEffect(i == current ? 1.1 : 1)
                                .frame(minWidth: 32, minHeight: 32)
                                .contentShape(Rectangle())
                        }
                        .buttonStyle(.plain)
                        .accessibilityLabel("\(i)")
                    }
                    Spacer(minLength: 0)
                }
                .renderInputChrome()
                .macRatingControl(current: current, count: count, label: c.label) { set($0, on: value) }
            }
        } else {
            NeedsForm(c: c)
        }
    }

    private func set(_ stars: Int, on value: Binding<Double>) {
        let next = Self.clamp(stars, count: NumericInput.ratingCount(c.max))
        guard next != NumericInput.ratingValue(value.wrappedValue, count: NumericInput.ratingCount(c.max)) else { return }
        withAnimation(RenderTheme.Motion.press) { value.wrappedValue = Double(next) }
    }

    /// 付けられる星の数(0 = 未評価 〜 count)。キーボードや VoiceOver の増減が端を越えないようにする。
    static func clamp(_ stars: Int, count: Int) -> Int { max(0, min(count, stars)) }
}

private extension View {
    /// Mac だけ: 星の列を 1 つの操作できる部品にする(Tab でフォーカス・← → と数字キー・VoiceOver の増減)。
    @ViewBuilder
    func macRatingControl(current: Int, count: Int, label: String?, set: @escaping (Int) -> Void) -> some View {
        #if os(macOS)
        focusable()
            .onKeyPress(.leftArrow) { set(RatingLeaf.clamp(current - 1, count: count)); return .handled }
            .onKeyPress(.rightArrow) { set(RatingLeaf.clamp(current + 1, count: count)); return .handled }
            .onKeyPress(.delete) { set(0); return .handled }
            .onKeyPress(characters: .decimalDigits, phases: .down) { press in
                guard let digit = press.characters.first.flatMap({ Int(String($0)) }), digit <= count else { return .ignored }
                set(digit)
                return .handled
            }
            // VoiceOver: 星 1 個ずつではなく「評価, 3/5」で読み、上下の矢印で増減する
            .accessibilityElement(children: .ignore)
            .accessibilityLabel(label ?? L("評価"))
            .accessibilityValue("\(current)/\(count)")
            .accessibilityAdjustableAction { direction in
                switch direction {
                case .increment: set(RatingLeaf.clamp(current + 1, count: count))
                case .decrement: set(RatingLeaf.clamp(current - 1, count: count))
                @unknown default: break
                }
            }
        #else
        self
        #endif
    }
}
