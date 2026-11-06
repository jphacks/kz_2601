import SwiftUI

/// 横向き(#625)のホスト側の寸法。iPhone の横向き = 高さが compact。
/// 方針: 縦と同じ部品を使い、横では「幅を絞って中央に寄せる」「大きな飾り(ツル・マイク)を小さくする」「縦に収まらない所はスクロール」。
/// 状態(入力中の文字・録音・開いているシート)は親の @State / AppRuntime にあるので、並べ方を変えても消えない。
enum LandscapeLayout {
    /// 文章・ボタンを読みやすく保つ最大幅(横向きのシート・チュートリアル・対話)
    static let readableWidth: CGFloat = 560

    static func isCompactHeight(_ verticalSizeClass: UserInterfaceSizeClass?) -> Bool {
        verticalSizeClass == .compact
    }

    /// 音声画面の大きなマイクの径。横向きは縦の空きが 300pt ほどなので小さく
    static func micSize(isInitial: Bool, compactHeight: Bool) -> CGFloat {
        switch (isInitial, compactHeight) {
        case (true, false): 132
        case (false, false): 104
        case (true, true): 96
        case (false, true): 84
        }
    }

    /// 音声画面の見出しの大きさ
    static func headingSize(compactHeight: Bool) -> CGFloat { compactHeight ? 24 : 30 }

    /// チュートリアルのツルの大きさ
    static func tutorialMascotSize(compactHeight: Bool) -> CGFloat { compactHeight ? 84 : 132 }
}

extension View {
    /// 横に広い時だけ幅を `LandscapeLayout.readableWidth` までにして中央に寄せる(縦向きの iPhone では何も変わらない)
    func readableWidth(_ width: CGFloat = LandscapeLayout.readableWidth) -> some View {
        frame(maxWidth: width).frame(maxWidth: .infinity)
    }
}
