import Foundation
import CoreGraphics

/// Mac で生成したアプリの FAB を出す試作(#2327・#1137 の判断の材料)。既定 off。
/// DEBUG ビルドで `defaults write <bundle id> metamon.debug.macFabGutter -bool YES`
/// (または起動引数 `-metamon.debug.macFabGutter YES`)の時だけ on。リリースビルドと iOS は常に off(FAB を出さない)。
public enum MacFabExperiment {
    public static let defaultsKey = "metamon.debug.macFabGutter"

    public static func isEnabled(_ defaults: UserDefaults = .standard) -> Bool {
        #if os(macOS) && DEBUG
        defaults.bool(forKey: defaultsKey)
        #else
        false
        #endif
    }
}

/// FAB の置き場所(中央の列を描く領域のローカル座標・左上原点)。View から切り離した純粋関数(テスト対象)。
/// - 左右の余白が `GridMetrics.fabGutter` 以上: 中央の列の右の余白の下(部品には重ならないので最下段の退避も末尾の余白も要らない)
/// - 余白が無い(窓が狭い): グリッドの右下。最下段の右端に部品があればその上へ逃がす(#83)。末尾の余白(#157)はホストが `fabFrame` を渡して足す
public enum MacFabPlacement {
    /// FAB の径(Mac はポインタで押すので iOS の 60 より小さく)
    public static let size: CGFloat = 52
    /// 窓の下端からの距離・列との間
    public static let edge: CGFloat = 24
    public static let gap: CGFloat = 16

    public struct Placement: Equatable, Sendable {
        public var frame: CGRect
        public var inGutter: Bool
    }

    /// - Parameters:
    ///   - container: FAB を置く領域の大きさ
    ///   - columnWidth: 中央の列の幅(`GridColumnWidthKey`)。まだ分からなければ領域の幅
    ///   - obstructionTop: 最下段の右端の部品の上端(領域のローカル y)。無ければ nil
    ///   - inset: 中央の列の内側の余白(`RenderTheme.Spacing.screenInset`)
    public static func placement(container: CGSize, columnWidth: CGFloat?, obstructionTop: CGFloat?, inset: CGFloat) -> Placement {
        let column = min(columnWidth ?? container.width, container.width)
        let columnRight = (container.width + column) / 2
        let sideMargin = (container.width - column) / 2
        let bottomY = container.height - edge - size
        if sideMargin >= GridMetrics.fabGutter {
            let x = min(columnRight + gap, container.width - edge - size)
            return Placement(frame: CGRect(x: x, y: bottomY, width: size, height: size), inGutter: true)
        }
        let x = max(0, columnRight - inset - size)
        let y = obstructionTop.map { min(bottomY, $0 - gap - size) } ?? bottomY
        return Placement(frame: CGRect(x: x, y: max(0, y), width: size, height: size), inGutter: false)
    }
}
