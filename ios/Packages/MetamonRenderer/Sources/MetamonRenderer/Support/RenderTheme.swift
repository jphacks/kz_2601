import SwiftUI

/// 生成アプリ側(Renderer の全部品)のデザイントークン。
/// アプリ本体 `ios/Metamon/App/Theme.swift` と**同じ値**を二重定義している(依存の向きが 本体→Renderer なので本体 Theme は import できない)。
/// 本体を変えたらここも揃える。対応表:
///   Theme.Spacing.{xs,sm,md,lg,xl} = RenderTheme.Spacing.{xs,sm,md,lg,xl}(4/8/16/24/32)
///   Theme.Radius.sm(10)/md(16)     = RenderTheme.Radius.control(10)/card(16)。Renderer の最大は surface(20)= 本体 card(22) より1段小さい(グリッドセルが小さいため)
///   Theme.Colors.canvas/surface/surfaceElevated/hairline = RenderTheme.Colors の同名(system grouped 色。ダーク/ライト自動)
///   Theme.Shadow.soft(0.10/18/8)   = RenderTheme.Shadow.card は 0.06/10/3(セルが密集するので控えめ。ダークではほぼ見えない前提)
public enum RenderTheme {
    public enum Spacing {
        public static let xs: CGFloat = 4
        public static let sm: CGFloat = 8
        public static let md: CGFloat = 16
        public static let lg: CGFloat = 24
        public static let xl: CGFloat = 32
        /// グリッドのセル間ギャップ(8〜12 の範囲で 10)
        public static let gridGap: CGFloat = 10
        /// 画面端の余白(グリッド外周)
        public static let screenInset: CGFloat = 14
        /// 入力部品の内側余白
        public static let inputH: CGFloat = 12
        public static let inputV: CGFloat = 10
        /// list の行の内側余白
        public static let row: CGFloat = 12
    }

    public enum Radius {
        /// 入力・小さなピル(Theme.Radius.sm)
        public static let control: CGFloat = 10
        /// list の行・写真・セル内クリップ
        public static let inner: CGFloat = 12
        /// カード(Theme.Radius.md)
        public static let card: CGFloat = 16
        /// 大きな面(セル全体・地図など)
        public static let surface: CGFloat = 20
    }

    public enum Colors {
        public static let canvas = Color(.systemGroupedBackground)
        public static let surface = Color(.secondarySystemGroupedBackground)
        public static let surfaceElevated = Color(.tertiarySystemGroupedBackground)
        /// 1pt の薄い枠線・セパレータ
        public static let hairline = Color.primary.opacity(0.08)
        public static let separator = Color(.separator)
        public static let positive = Color.green
        public static let negative = Color.red
        public static let warning = Color.orange
    }

    public enum Shadow {
        public static let cardColor = Color.black.opacity(0.06)
        public static let cardRadius: CGFloat = 10
        public static let cardY: CGFloat = 3
    }

    public enum Fonts {
        /// stat の数値(大きく・等幅数字・rounded)
        public static let statValue = Font.system(.title, design: .rounded).weight(.bold).monospacedDigit()
        public static let statLabel = Font.footnote.weight(.medium)
        public static let cardTitle = Font.headline
        /// section 見出し(小さめ・secondary・少し詰める)
        public static let sectionTitle = Font.footnote.weight(.semibold)
        public static let tableHeader = Font.caption.weight(.semibold)
        public static let tableCell = Font.subheadline.monospacedDigit()
        public static let rowTitle = Font.body.weight(.medium)
        public static let rowSubtitle = Font.caption
        public static let rowTrailing = Font.body.weight(.semibold).monospacedDigit()
        public static let button = Font.body.weight(.semibold)
        public static let inputLabel = Font.caption.weight(.medium)
    }

    public enum Motion {
        /// 変身直後のセル出現(index × stagger 遅延)
        public static let appear = Animation.easeOut(duration: 0.32)
        public static let stagger: Double = 0.03
        public static let appearScale: CGFloat = 0.98
    }
}

// MARK: - 共通モディファイア

extension View {
    /// 面(カード)。surface + card 角丸 + hairline 枠 + 控えめな影。`padding: 0` で中身がフチまで届く list/table 用。
    func renderCard(padding: CGFloat = RenderTheme.Spacing.md, radius: CGFloat = RenderTheme.Radius.card) -> some View {
        self
            .padding(padding)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(RenderTheme.Colors.surface, in: RoundedRectangle(cornerRadius: radius, style: .continuous))
            .overlay(RoundedRectangle(cornerRadius: radius, style: .continuous).strokeBorder(RenderTheme.Colors.hairline))
            .shadow(color: RenderTheme.Shadow.cardColor, radius: RenderTheme.Shadow.cardRadius, y: RenderTheme.Shadow.cardY)
    }

    /// 入力(textField / numberField / secureField / textEditor)の器。カードの中に置かれても一段沈んで見える。
    func renderInputChrome(padding: Bool = true) -> some View {
        self
            .padding(.horizontal, padding ? RenderTheme.Spacing.inputH : 0)
            .padding(.vertical, padding ? RenderTheme.Spacing.inputV : 0)
            .background(RenderTheme.Colors.surfaceElevated, in: RoundedRectangle(cornerRadius: RenderTheme.Radius.control, style: .continuous))
            .overlay(RoundedRectangle(cornerRadius: RenderTheme.Radius.control, style: .continuous).strokeBorder(RenderTheme.Colors.hairline))
    }
}

/// 1pt の薄いセパレータ(list / table の行間)。
struct Hairline: View {
    var inset: CGFloat = 0
    var body: some View {
        Rectangle()
            .fill(RenderTheme.Colors.hairline)
            .frame(height: 1)
            .padding(.leading, inset)
    }
}

/// 空・読み込み中のプレースホルダ(`emptyState` 風)。list / table / timeline / chart が共用。
struct EmptyPlaceholder: View {
    var icon: String = "tray"
    var text: String
    var loading = false

    var body: some View {
        VStack(spacing: RenderTheme.Spacing.sm) {
            if loading {
                ProgressView().controlSize(.regular)
            } else {
                Image(systemName: icon)
                    .font(.title2)
                    .foregroundStyle(.tertiary)
            }
            Text(text)
                .font(.subheadline)
                .foregroundStyle(.secondary)
                .multilineTextAlignment(.center)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity) // 器(セル)が高さを渡してくれば全体を使い中央に置く(#84)
        .padding(.vertical, RenderTheme.Spacing.lg)
        .padding(.horizontal, RenderTheme.Spacing.md)
        .background(
            RoundedRectangle(cornerRadius: RenderTheme.Radius.card, style: .continuous)
                .strokeBorder(RenderTheme.Colors.hairline, style: StrokeStyle(lineWidth: 1, dash: [6, 5]))
        )
    }
}
