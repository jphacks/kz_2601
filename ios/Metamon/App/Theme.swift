import SwiftUI
import MetamonRenderer

/// アプリ本体(音声画面・サインイン・生成中・設定)のデザイントークン。Renderer の部品の見た目はここでは扱わない。
/// 色はライト/ダーク両対応。値の正典は Renderer の `RenderTheme.Colors`(本体と生成アプリを同じトーンに揃える・#102)。
enum Theme {
    enum Spacing {
        static let xs: CGFloat = 4
        static let sm: CGFloat = 8
        static let md: CGFloat = 16
        static let lg: CGFloat = 24
        static let xl: CGFloat = 32
        static let xxl: CGFloat = 48
    }

    enum Radius {
        static let sm: CGFloat = 10
        static let md: CGFloat = 16
        static let card: CGFloat = 22
        static let lg: CGFloat = 28
    }

    enum Colors {
        /// ブランド(Tailor It = アイコンの紺に合わせた藍〜青緑)
        static let brandA = RenderTheme.Colors.brandA
        static let brandB = RenderTheme.Colors.brandB
        /// 文字に使うブランド色(AA)
        static let brandText = RenderTheme.Colors.brandText
        /// 録音中(赤〜珊瑚)
        static let recordA = Color(hex: "#FF5F6D")!
        static let recordB = Color(hex: "#FF9966")!
        /// 警告(生成が長引いた時)
        static let warning = Color.orange

        static let canvas = RenderTheme.Colors.canvas
        static let surface = RenderTheme.Colors.surface
        static let surfaceElevated = RenderTheme.Colors.surfaceElevated
        static let hairline = RenderTheme.Colors.hairline
    }

    /// ランタイム側オーバーレイ(DSL 外)の配置。
    enum Layout {
        /// マイク FAB の位置。DSL の「追加」ボタンは右下(`D11:F12` が定番)に来るので**左下**に置いて被らないようにする。
        static let fabAlignment: Alignment = .bottomLeading
        static let fabSize: CGFloat = 60
        static let fabHorizontalInset: CGFloat = Spacing.lg
        /// タブバーがある時はその上に逃がす。
        static func fabBottomInset(hasTabs: Bool) -> CGFloat { hasTabs ? 70 : Spacing.lg }
        /// FAB 退避(#83): 最下段(行 11〜12・A〜B 列)に DSL の部品がある時は、その上端より `fabClearanceGap` 上に逃がす。
        /// `obstructionTop` = 部品の上端(global y・Renderer の `GridBottomObstructionKey`)、`rootBottom` = FAB を置く領域の下端(global y)。
        static let fabClearanceGap: CGFloat = Spacing.sm
        static func fabBottomInset(hasTabs: Bool, obstructionTop: CGFloat?, rootBottom: CGFloat) -> CGFloat {
            let base = fabBottomInset(hasTabs: hasTabs)
            guard let obstructionTop else { return base }
            return max(base, rootBottom - obstructionTop + fabClearanceGap)
        }
        /// フォーム画面(`screen.entity` あり)では FAB を隠し、ナビバー右上の小さなマイク(この径)を入口にする。
        static let toolbarMicSize: CGFloat = 30
        /// 長押し(= 変身一覧)と判定するまでの秒数。初期画面の大きなマイクも同じ。
        static let fabLongPressSeconds: Double = 0.45
    }

    enum Shadow {
        /// 紺を薄く落とした影(真っ黒より生成りの面に馴染む)
        static let softColor = Color(red: 15 / 255, green: 26 / 255, blue: 34 / 255).opacity(0.10)
        static let softRadius: CGFloat = 18
        static let softY: CGFloat = 8
    }

    static let brandGradient = LinearGradient(
        colors: [Colors.brandA, Colors.brandB],
        startPoint: .topLeading, endPoint: .bottomTrailing
    )
    static let recordGradient = LinearGradient(
        colors: [Colors.recordA, Colors.recordB],
        startPoint: .topLeading, endPoint: .bottomTrailing
    )
    static let brandAngular = AngularGradient(
        colors: [Colors.brandA, Colors.brandB, Colors.brandA.opacity(0.6), Colors.brandA],
        center: .center
    )

    /// 音声画面の背景(淡いブランド色の滲み + キャンバス)
    static func backdrop(_ scheme: ColorScheme) -> some View {
        ZStack {
            Colors.canvas
            RadialGradient(
                colors: [Colors.brandA.opacity(scheme == .dark ? 0.28 : 0.18), .clear],
                center: .init(x: 0.2, y: 0.1), startRadius: 0, endRadius: 420
            )
            RadialGradient(
                colors: [Colors.brandB.opacity(scheme == .dark ? 0.22 : 0.16), .clear],
                center: .init(x: 0.9, y: 0.9), startRadius: 0, endRadius: 380
            )
        }
        .ignoresSafeArea()
    }
}

extension View {
    /// 面(カード)。surface 色 + card 角丸 + 柔らかい影。
    func metamonCard(padding: CGFloat = Theme.Spacing.md) -> some View {
        self
            .padding(padding)
            .background(Theme.Colors.surface, in: RoundedRectangle(cornerRadius: Theme.Radius.card, style: .continuous))
            .overlay(RoundedRectangle(cornerRadius: Theme.Radius.card, style: .continuous).strokeBorder(Theme.Colors.hairline))
            .shadow(color: Theme.Shadow.softColor, radius: Theme.Shadow.softRadius, y: Theme.Shadow.softY)
    }

    /// ブランドグラデーションの文字。
    func brandText() -> some View {
        self.foregroundStyle(Theme.brandGradient)
    }
}

/// ブランドマーク(サインイン・設定で共用)。
struct BrandMark: View {
    var size: CGFloat = 72

    var body: some View {
        ZStack {
            Circle().fill(Theme.brandGradient)
            Image(systemName: "waveform")
                .font(.system(size: size * 0.46, weight: .semibold))
                .foregroundStyle(.white)
        }
        .frame(width: size, height: size)
        .shadow(color: Theme.Colors.brandA.opacity(0.35), radius: size * 0.25, y: size * 0.1)
    }
}
