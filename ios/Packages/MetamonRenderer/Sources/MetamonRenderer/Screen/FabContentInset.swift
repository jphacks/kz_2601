import SwiftUI

/// マイク FAB の下に隠れた中身をスクロールで逃がす(#157)。
/// FAB は「最下段に寄せた操作部品」だけ避けて上に逃げる(#83)。背の高い list / table / timeline は背景扱いで FAB が上に浮くので、
/// 中身の末尾に FAB ぶんの余白を足し、最後の行を FAB の上までスクロールできるようにする。
public extension FabClearance {
    /// FAB の上端と最後の行の間に空ける距離。
    static let contentGap: CGFloat = RenderTheme.Spacing.sm

    /// スクロール領域 `viewport`(global)の末尾に足す余白。FAB と横に重ならない / FAB が領域より下 or 上 / FAB が無い(隠れている・非表示)なら 0。
    /// FAB を上へドラッグして領域の途中にある時も、その上端まで最後の行を持ち上げられるだけの余白を返す。
    /// ただし FAB の上に `minVisible` も残らない(低い器を FAB が丸ごと覆う)時は、持ち上げても行が見えないので 0(空白までスクロールさせない)。
    static let minVisible: CGFloat = 32

    static func contentMargin(viewport: CGRect, fab: CGRect?, gap: CGFloat = contentGap) -> CGFloat {
        guard let fab, !viewport.isEmpty, !fab.isEmpty,
              fab.maxX > viewport.minX, fab.minX < viewport.maxX,
              fab.minY < viewport.maxY, fab.maxY > viewport.minY,
              fab.minY - gap - viewport.minY >= minVisible else { return 0 }
        return (viewport.maxY - fab.minY + gap).rounded(.up)
    }
}

private struct FabFrameKey: EnvironmentKey {
    static let defaultValue: CGRect? = nil
}

public extension EnvironmentValues {
    /// ホストが重ねているマイク FAB の矩形(global)。隠れている / 出していない時は nil(#157)。
    var fabFrame: CGRect? {
        get { self[FabFrameKey.self] }
        set { self[FabFrameKey.self] = newValue }
    }
}
