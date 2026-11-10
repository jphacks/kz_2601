import CoreGraphics
import MetamonDSL

/// 画面グリッドの寸法(#625)。縦向きは従来どおり「可視幅 ÷ 6 × 可視高さ ÷ 12」で画面にぴったり収める。
/// 横向き(高さが compact)は高さ ÷ 12 だと行が潰れて読めないので、
/// - 幅は読みやすい最大幅までにして中央に寄せ、左右の余白にマイク FAB が収まるようにする(FAB が部品に重ならない)
/// - 行の高さは縦向きの iPhone と同じくらいに固定し、はみ出した分は画面ごと縦スクロール
/// 列の割り付け(A〜F)は縦と同じ。2 カラム化などの並べ替えはしない(DSL の area をそのまま読む)。
public struct GridMetrics: Equatable, Sendable {
    /// グリッドの外枠の幅(画面端余白 `inset` を含む)。縦向きは画面の幅いっぱい
    public var frameWidth: CGFloat
    /// 1 列・1 行の大きさ
    public var cellW: CGFloat
    public var cellH: CGFloat
    /// 高さに収めず、画面ごと縦スクロールさせるか(横向き)。voiceInput 画面もスクロールさせる
    public var scrollsVertically: Bool
    /// 左右の余白がマイク FAB より広く、FAB が部品に重ならないか(最下段の退避・末尾の余白が要らない)
    public var fabInGutter: Bool

    /// 横向きのグリッドの外枠の最大幅(読みやすい幅。縦向きの最大の iPhone の 1.5 倍ほど)
    public static let maxLandscapeWidth: CGFloat = 640
    /// 横向きの 1 行の高さ(縦向きの iPhone 15 のタブ付き画面の行 ≒ 52pt と同じ)
    public static let landscapeCellHeight: CGFloat = 52
    /// 左右の余白に取りたい幅 = FAB の端からの距離(24)+ 径(60)+ すき間(12)。ホストの `Theme.Layout.fabSize` / `fabHorizontalInset` と合わせる
    public static let fabGutter: CGFloat = 96
    /// FAB のための余白を取ると枠がこれより狭くなる時は、余白を諦めて幅を優先する(SE など)
    public static let minLandscapeWidth: CGFloat = 420

    /// - Parameters:
    ///   - width: グリッドを置く領域の幅(セーフエリアの内側)
    ///   - height: 行の高さの基準にする高さ(キーボードで縮む前の高さ・`GridLayoutHeight`)
    ///   - compactHeight: 高さが compact(iPhone の横向き)か
    public static func resolve(width: CGFloat, height: CGFloat, inset: CGFloat, compactHeight: Bool) -> GridMetrics {
        let columns = CGFloat(GridArea.columns)
        guard compactHeight else {
            return GridMetrics(
                frameWidth: width,
                cellW: max(0, width - inset * 2) / columns,
                cellH: max(0, height - inset * 2) / CGFloat(GridArea.visibleRows),
                scrollsVertically: false,
                fabInGutter: false
            )
        }
        let withGutter = min(width - fabGutter * 2, maxLandscapeWidth)
        let frameWidth = withGutter >= minLandscapeWidth ? withGutter : min(width, maxLandscapeWidth)
        let sideMargin = (width - frameWidth) / 2
        return GridMetrics(
            frameWidth: frameWidth,
            cellW: max(0, frameWidth - inset * 2) / columns,
            // 横向きでも高さが十分ある領域(外部ディスプレイ等)では縦向きと同じく高さで割る
            cellH: max(landscapeCellHeight, max(0, height - inset * 2) / CGFloat(GridArea.visibleRows)),
            scrollsVertically: true,
            fabInGutter: sideMargin >= fabGutter
        )
    }

    // MARK: - Mac(#938)

    /// Mac のグリッドの外枠の幅の下限・上限。下限は Web の共有ページの最大幅(480px)と同じ、上限は iPad 縦向きの幅の手前
    public static let minDesktopWidth: CGFloat = 480
    public static let maxDesktopWidth: CGFloat = 720
    /// Mac で外枠の幅を高さから決める比率(幅 = 高さ × 3/4)。iPad の縦向き(3:4)と同じ縦長に保つ
    public static let desktopAspect: CGFloat = 3.0 / 4.0

    /// Mac のウィンドウ(横長で大きさが自由に変わる)の寸法。列は 6 のまま(DSL の area をそのまま読む)。
    /// - 幅は「高さ × 3/4」を 480〜720 に収め、領域より広ければ中央に寄せる(横に引き伸ばさない)
    /// - 行の高さは縦向きと同じく高さ ÷ 12。低いウィンドウで iOS の横向きの行の高さ(52)を下回る時はそれで止め、はみ出す分は縦スクロール
    /// - マイク FAB は中央の列の外(ツールバー)にあるので、余白に FAB を収める計算はしない
    public static func resolveDesktop(width: CGFloat, height: CGFloat, inset: CGFloat) -> GridMetrics {
        let preferred = min(max(height * desktopAspect, minDesktopWidth), maxDesktopWidth)
        let frameWidth = min(width, preferred)
        let fitted = max(0, height - inset * 2) / CGFloat(GridArea.visibleRows)
        return GridMetrics(
            frameWidth: frameWidth,
            cellW: max(0, frameWidth - inset * 2) / CGFloat(GridArea.columns),
            cellH: max(landscapeCellHeight, fitted),
            scrollsVertically: fitted < landscapeCellHeight,
            fabInGutter: false
        )
    }
}

#if os(macOS)
import SwiftUI

/// Mac の中央の列(グリッドの外枠)の幅(#1337)。画面下の保存バーを同じ幅に収めるために `GridScreenLayout` が上げる。
struct GridColumnWidthKey: PreferenceKey {
    static let defaultValue: CGFloat? = nil
    static func reduce(value: inout CGFloat?, nextValue: () -> CGFloat?) {
        value = nextValue() ?? value
    }
}
#endif
