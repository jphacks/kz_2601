import SwiftUI

/// 上下に出る通知(要約バナー・お知らせ・新着・トースト)をスワイプで閉じる判定(#628)。
/// 近い端の向き(上の通知は上・下のトーストは下)に払うと閉じ、途中で離すと元の位置に戻る。
enum SwipeDismiss {
    /// これだけ端の向きへ動かして離したら閉じる(pt)
    static let distance: CGFloat = 36
    /// 短くても速く払った時: 指を離した後の予測位置がここまで届けば閉じる(pt)
    static let flingDistance: CGFloat = 110
    /// 逆向き(画面の内側)へは少しだけ動いて止まる(pt)
    static let maxPull: CGFloat = 12

    /// 端の向きを正にした移動量。
    static func toward(_ translation: CGFloat, edge: VerticalEdge) -> CGFloat {
        edge == .top ? -translation : translation
    }

    /// 指を離した時に閉じるか。
    static func shouldDismiss(translation: CGFloat, predicted: CGFloat, edge: VerticalEdge) -> Bool {
        toward(translation, edge: edge) >= distance || toward(predicted, edge: edge) >= flingDistance
    }

    /// 動かしている間の表示位置(y)。端の向きは指に付いていき、逆向きは抵抗を付けて `maxPull` まで。
    static func offset(translation: CGFloat, edge: VerticalEdge) -> CGFloat {
        let t = toward(translation, edge: edge)
        let shown = t >= 0 ? t : -min(maxPull, -t / 4)
        return edge == .top ? -shown : shown
    }

    /// 端へ動かすほど薄くする(閉じる手前で半分)。
    static func opacity(translation: CGFloat, edge: VerticalEdge) -> Double {
        let t = max(0, toward(translation, edge: edge))
        return Double(max(0.4, 1 - t / (distance * 4)))
    }
}

private struct SwipeToDismissModifier: ViewModifier {
    let edge: VerticalEdge
    let onDismiss: () -> Void
    @State private var drag: CGFloat = 0

    func body(content: Content) -> some View {
        content
            .offset(y: SwipeDismiss.offset(translation: drag, edge: edge))
            .opacity(SwipeDismiss.opacity(translation: drag, edge: edge))
            // 中のボタン(元に戻す・詳しく見る・×)のタップは残す。ボタンの上から払っても閉じる
            .simultaneousGesture(
                DragGesture(minimumDistance: 8)
                    .onChanged { drag = $0.translation.height }
                    .onEnded { value in
                        if SwipeDismiss.shouldDismiss(translation: value.translation.height,
                                                      predicted: value.predictedEndTranslation.height, edge: edge) {
                            onDismiss()
                        } else {
                            withAnimation(.spring(duration: 0.3, bounce: 0.25)) { drag = 0 }
                        }
                    }
            )
    }
}

extension View {
    /// 通知をスワイプで閉じる(#628)。`edge` は通知が出ている端(その向きへ払うと閉じる)。自動で消える秒数は呼び出し側のまま。
    func swipeToDismiss(edge: VerticalEdge = .top, onDismiss: @escaping () -> Void) -> some View {
        modifier(SwipeToDismissModifier(edge: edge, onDismiss: onDismiss))
    }

    /// VoiceOver の操作「閉じる」(#628)。読み上げる要素(accessibility element)に付ける。
    func accessibilityDismissAction(_ onDismiss: @escaping () -> Void) -> some View {
        accessibilityAction(named: Text("閉じる"), onDismiss)
    }
}
