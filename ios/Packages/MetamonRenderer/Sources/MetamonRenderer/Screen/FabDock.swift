import CoreGraphics

/// マイク FAB のピクチャーインピクチャー風の吸着・隠す判定(#52)。View から切り離した純粋関数(テスト対象)。
/// 座標は FAB を置く領域のローカル座標(左上原点)。「下端距離」= 領域の下端から FAB の下端まで。
public enum FabDock {
    /// これ以上動いたらドラッグ(= タップ・長押しは失敗)。`LongPressGesture` の既定の許容移動と揃える。
    public static let dragThreshold: CGFloat = 10
    /// 離した時の中心が端からこの割合 × FAB 径 以内なら隠す。
    public static let hideFraction: CGFloat = 0.3
    /// 隠れた取っ手を内側へこれ以上引いたら戻す。
    public static let revealDistance: CGFloat = 20
    /// 保存値がこれなら「自動」(従来の左下 + 部品退避のまま)。
    public static let autoBottom: CGFloat = -1

    public struct Snap: Equatable, Sendable {
        public var side: FabSide
        public var hidden: Bool
        public init(side: FabSide, hidden: Bool) {
            self.side = side
            self.hidden = hidden
        }
    }

    /// 指を離した時の吸着先。`centerX` = 実位置の中心 x、`predictedCenterX` = 慣性込みの予測位置の中心 x。
    /// 辺は予測位置で選ぶ(端へ弾いた方向を優先)。隠すのは、実位置か予測位置のどちらかが選んだ辺から `fabSize * hideFraction` 以内(はみ出しを含む)の時。
    public static func snap(centerX: CGFloat, predictedCenterX: CGFloat, width: CGFloat, fabSize: CGFloat) -> Snap {
        let side: FabSide = predictedCenterX < width / 2 ? .leading : .trailing
        let threshold = fabSize * hideFraction
        func edgeDistance(_ x: CGFloat) -> CGFloat { side == .leading ? x : width - x }
        let hidden = edgeDistance(centerX) <= threshold || edgeDistance(predictedCenterX) <= threshold
        return Snap(side: side, hidden: hidden)
    }

    /// 取っ手のドラッグ量(x)で FAB を戻すか。内側 = 左の取っ手なら右、右の取っ手なら左。
    public static func shouldReveal(translationX: CGFloat, side: FabSide) -> Bool {
        (side == .leading ? translationX : -translationX) >= revealDistance
    }

    /// 表示する下端距離。自動(負値)なら `autoInset`(タブバー・部品退避込み)。
    /// ユーザー値は `autoInset` より下には置かない(部品に被せない)、`maxBottom`(ナビバーにかからない上限)より上にも置かない。
    public static func displayedBottom(stored: CGFloat, autoInset: CGFloat, maxBottom: CGFloat) -> CGFloat {
        guard stored >= 0 else { return autoInset }
        return max(autoInset, min(stored, maxBottom))
    }

    /// ドラッグを離した位置(FAB 中心 y)から保存する下端距離を求める。`minBottom`〜`maxBottom` に収める。
    public static func storedBottom(centerY: CGFloat, height: CGFloat, fabSize: CGFloat, minBottom: CGFloat, maxBottom: CGFloat) -> CGFloat {
        let raw = height - centerY - fabSize / 2
        return max(minBottom, min(raw, max(minBottom, maxBottom)))
    }
}
