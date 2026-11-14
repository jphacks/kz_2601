import SwiftUI

/// Mac の本体の画面の寸法と文字(#935)。色は iOS の `Theme`(値の正典は Renderer の `RenderTheme`)をそのまま使い、Mac だけの色は作らない(例外はコントラストを上げる時の濃い枠 `MacTheme.Colors.strongHairline`・#1206)。
/// 文字は text style のまま(Mac では body = 13pt など Mac の大きさになる)。余白は Mac のウィンドウの決まり(端 20pt)に合わせる。
/// アクセントは Brand.xcassets の AccentColor(藍)。システム設定でアクセントを選んでいる人にはそちらが効くので、`.tint` で上書きしない。
enum MacTheme {
    enum Spacing {
        /// ウィンドウ・中央の列の端からの余白
        static let window: CGFloat = 20
        /// 見出しと本文などのまとまりの間
        static let group: CGFloat = 12
        /// サイドバーの行の名前と説明の間
        static let rowLine: CGFloat = 2
    }

    enum Fonts {
        static let emptyTitle = Font.title2.weight(.semibold)
        static let emptyBody = Font.body
        static let sidebarTitle = Font.body
        static let sidebarDetail = Font.subheadline
    }

    enum Mascot {
        /// 中央の列が空の時のツル
        static let empty: CGFloat = 112
        /// 設定ウィンドウ・サイドバーが空の時のツル
        static let small: CGFloat = 72
    }
}

/// ツル + 見出し + 説明。中央の列やサイドバーが空の時に `ContentUnavailableView` の代わりに出す。
/// 背景は iOS の音声画面と同じブランドの滲み(`Theme.backdrop`)。サイドバーの中では材質を活かすため背景を敷かない。
struct MacEmptyState: View {
    let pose: CraneMascot.Pose
    let title: LocalizedStringKey
    var message: LocalizedStringKey?
    var mascotSize: CGFloat = MacTheme.Mascot.empty
    var showsBackdrop = true

    @Environment(\.colorScheme) private var scheme

    var body: some View {
        VStack(spacing: MacTheme.Spacing.group) {
            CraneMascot(pose: pose, size: mascotSize)
            Text(title)
                .font(MacTheme.Fonts.emptyTitle)
            if let message {
                Text(message)
                    .font(MacTheme.Fonts.emptyBody)
                    .macSecondaryText() // コントラストを上げる時は本文の色(#1206)
                    .multilineTextAlignment(.center)
            }
        }
        .padding(MacTheme.Spacing.window)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background {
            if showsBackdrop { Theme.backdrop(scheme) }
        }
        .accessibilityElement(children: .combine)
    }
}
