import SwiftUI

/// 値の文字。基準の大きさ(標準の文字サイズでの pt)を、ユーザーの文字の大きさ設定に合わせて拡大縮小する。
/// ViewThatFits の候補に並べて使う(候補ごとに基準が違っても、同じ倍率で動く)。
struct ScaledValueText: View {
    private let text: String
    private let weight: Font.Weight
    private let monospacedDigit: Bool
    @ScaledMetric private var size: CGFloat

    init(_ text: String, size: CGFloat, weight: Font.Weight = .bold, monospacedDigit: Bool = true, relativeTo style: Font.TextStyle = .largeTitle) {
        self.text = text
        self.weight = weight
        self.monospacedDigit = monospacedDigit
        _size = ScaledMetric(wrappedValue: size, relativeTo: style)
    }

    var body: some View {
        let font = Font.system(size: size, weight: weight, design: .rounded)
        Text(text).font(monospacedDigit ? font.monospacedDigit() : font)
    }
}
