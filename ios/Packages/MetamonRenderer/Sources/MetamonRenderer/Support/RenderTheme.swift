import SwiftUI

/// 生成アプリ側(Renderer の全部品)のデザイントークン。方針は `docs/ios/design-system.md`。
/// アプリ本体 `ios/Metamon/App/Theme.swift` と**同じ値**を二重定義している(依存の向きが 本体→Renderer なので本体 Theme は import できない)。
/// 本体を変えたらここも揃える。対応表:
///   Theme.Colors.brandA/brandB     = RenderTheme.Colors.brandA/brandB(#8E6CFF / #4CC9FF)。生成アプリは `app.theme.primaryColor` を使わず常にこのブランド
///   Theme.Spacing.{xs,sm,md,lg,xl} = RenderTheme.Spacing.{xs,sm,md,lg,xl}(4/8/16/24/32)
///   Theme.Radius.md(16)            = RenderTheme.Radius.card(16)。control=12 / button=14 / sheet=24(本体 card 22 と lg 28 の間)
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
        public static let inputH: CGFloat = 14
        public static let inputV: CGFloat = 12
        /// フォームの入力同士の間隔
        public static let formGap: CGFloat = 12
        /// list の行の内側余白
        public static let row: CGFloat = 12
    }

    public enum Radius {
        /// 入力・小さなピル
        public static let control: CGFloat = 12
        /// list の行・写真サムネ・セル内クリップ
        public static let inner: CGFloat = 10
        /// ボタン
        public static let button: CGFloat = 14
        /// カード(Theme.Radius.md)
        public static let card: CGFloat = 16
        /// 大きな面(セル全体・地図・シート)
        public static let sheet: CGFloat = 24
        /// 互換: 旧名(セル全体)
        public static let surface: CGFloat = 20
    }

    public enum Colors {
        /// ブランド(Tailor It = 紫〜水色)。本体 Theme.Colors.brandA/brandB と同値
        public static let brandA = Color(hex: "#8E6CFF")!
        public static let brandB = Color(hex: "#4CC9FF")!
        /// ブランドの薄い面(secondary ボタン・選択行・アイコン背景)
        public static let brandSoft = brandA.opacity(0.12)
        public static let brandSoftStroke = brandA.opacity(0.22)

        public static let canvas = Color(.systemGroupedBackground)
        public static let surface = Color(.secondarySystemGroupedBackground)
        public static let surfaceElevated = Color(.tertiarySystemGroupedBackground)
        /// 1pt の薄い枠線・セパレータ
        public static let hairline = Color.primary.opacity(0.08)
        public static let separator = Color(.separator)
        public static let positive = Color.green
        public static let negative = Color.red
        public static let warning = Color.orange

        /// chart / カテゴリ色ドット。先頭 2 色がブランド、以降は systemColor(ダークで映える)
        public static let chartPalette: [Color] = [brandA, brandB, .mint, .indigo, .orange, .pink, .teal, .purple]

        /// 文字列(カテゴリ名など)→ パレットの安定した 1 色。
        public static func category(_ key: String) -> Color {
            var h: UInt32 = 2166136261
            for b in key.utf8 { h = (h ^ UInt32(b)) &* 16777619 }
            return chartPalette[Int(h % UInt32(chartPalette.count))]
        }
    }

    public enum Shadow {
        public static let cardColor = Color.black.opacity(0.06)
        public static let cardRadius: CGFloat = 10
        public static let cardY: CGFloat = 3
        /// primary ボタンのブランド影
        public static let brandColor = Colors.brandA.opacity(0.28)
        public static let brandRadius: CGFloat = 12
        public static let brandY: CGFloat = 6
    }

    public enum Fonts {
        /// stat の数値(大きく・等幅数字・rounded)
        public static let statValue = Font.system(.title, design: .rounded).weight(.bold).monospacedDigit()
        public static let statLabel = Font.footnote.weight(.medium)
        /// numberField の入力値(金額が主役)
        public static let number = Font.system(.title2, design: .rounded).weight(.semibold).monospacedDigit()
        public static let numberPrefix = Font.system(.title3, design: .rounded).weight(.medium)
        public static let cardTitle = Font.headline
        /// section 見出し(小さめ・secondary・少し詰める)
        public static let sectionTitle = Font.footnote.weight(.semibold)
        public static let tableHeader = Font.caption.weight(.semibold)
        public static let tableCell = Font.subheadline.monospacedDigit()
        public static let rowTitle = Font.body.weight(.semibold)
        public static let rowSubtitle = Font.caption
        public static let rowTrailing = Font.body.weight(.semibold).monospacedDigit()
        public static let button = Font.body.weight(.semibold)
        public static let inputLabel = Font.caption.weight(.medium)
        public static let inputValue = Font.body
        public static let chip = Font.subheadline.weight(.semibold)
        public static let legend = Font.caption
    }

    public enum Motion {
        /// 変身直後のセル出現(index × stagger 遅延)
        public static let appear = Animation.easeOut(duration: 0.32)
        public static let stagger: Double = 0.03
        public static let appearScale: CGFloat = 0.98
        /// 押下(ボタン・チップ・星)
        public static let press = Animation.spring(response: 0.28, dampingFraction: 0.7)
        public static let pressScale: CGFloat = 0.97
        /// 選択の移動(segmented / toggle)
        public static let select = Animation.snappy(duration: 0.25)
        /// 保存成功のチェック表示秒数・新規行ハイライト秒数
        public static let checkSeconds: Double = 0.6
        public static let highlightSeconds: Double = 1.2
    }

    /// ブランドグラデーション(primary ボタン・選択チップ・塗り星・ゲージ)
    public static let brandGradient = LinearGradient(
        colors: [Colors.brandA, Colors.brandB],
        startPoint: .topLeading, endPoint: .bottomTrailing
    )
    /// カードの左上に敷く極薄のブランド(stat)
    public static let brandWash = LinearGradient(
        colors: [Colors.brandA.opacity(0.10), Colors.brandB.opacity(0.03), .clear],
        startPoint: .topLeading, endPoint: .bottomTrailing
    )
}

// MARK: - 共通モディファイア

extension View {
    /// 面(カード)。surface + card 角丸 + hairline 枠 + 控えめな影。`padding: 0` で中身がフチまで届く list/table 用。
    /// `wash: true` で左上に極薄のブランドグラデーションを敷く(stat)。
    func renderCard(padding: CGFloat = RenderTheme.Spacing.md, radius: CGFloat = RenderTheme.Radius.card, wash: Bool = false) -> some View {
        let shape = RoundedRectangle(cornerRadius: radius, style: .continuous)
        return self
            .padding(padding)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(wash ? AnyShapeStyle(RenderTheme.brandWash) : AnyShapeStyle(.clear), in: shape)
            .background(RenderTheme.Colors.surface, in: shape)
            .overlay(shape.strokeBorder(RenderTheme.Colors.hairline))
            .shadow(color: RenderTheme.Shadow.cardColor, radius: RenderTheme.Shadow.cardRadius, y: RenderTheme.Shadow.cardY)
    }

    /// 入力(textField / numberField / secureField / textEditor / picker / datePicker…)の器。カードの中に置かれても一段沈んで見える。
    /// `focused` でブランド色のリング、`error` で赤のリング。
    func renderInputChrome(padding: Bool = true, focused: Bool = false, error: Bool = false) -> some View {
        let shape = RoundedRectangle(cornerRadius: RenderTheme.Radius.control, style: .continuous)
        let ring: Color = error ? RenderTheme.Colors.negative : (focused ? RenderTheme.Colors.brandA : RenderTheme.Colors.hairline)
        return self
            .padding(.horizontal, padding ? RenderTheme.Spacing.inputH : 0)
            .padding(.vertical, padding ? RenderTheme.Spacing.inputV : 0)
            .background(RenderTheme.Colors.surfaceElevated, in: shape)
            .overlay(shape.strokeBorder(ring, lineWidth: focused || error ? 1.5 : 1))
            .animation(RenderTheme.Motion.select, value: focused)
    }

    /// 押下 0.97 のスプリング(Button に `.buttonStyle(.plain)` を付けた上で使う簡易版)。
    func pressable(_ pressed: Bool) -> some View {
        self
            .scaleEffect(pressed ? RenderTheme.Motion.pressScale : 1)
            .animation(RenderTheme.Motion.press, value: pressed)
    }
}

/// 入力の左に置くアイコン(secondary、フォーカス中はブランド色)。
struct InputIcon: View {
    let name: String
    var focused = false
    var body: some View {
        Image(systemName: name)
            .font(.body.weight(.medium))
            .foregroundStyle(focused ? AnyShapeStyle(RenderTheme.Colors.brandA) : AnyShapeStyle(.secondary))
            .frame(width: 22)
            .accessibilityHidden(true)
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
/// `cta` があればブランド色の薄い 1 行を添える(「最初の 1 件を追加」など)。
struct EmptyPlaceholder: View {
    var icon: String = "tray"
    var text: String
    var cta: String? = nil
    var loading = false

    var body: some View {
        VStack(spacing: RenderTheme.Spacing.sm) {
            if loading {
                ProgressView().controlSize(.regular).tint(RenderTheme.Colors.brandA)
            } else {
                Image(systemName: icon)
                    .font(.title2.weight(.medium))
                    .foregroundStyle(RenderTheme.Colors.brandA.opacity(0.55))
                    .frame(width: 44, height: 44)
                    .background(RenderTheme.Colors.brandSoft, in: Circle())
            }
            Text(text)
                .font(.subheadline.weight(.medium))
                .foregroundStyle(.secondary)
                .multilineTextAlignment(.center)
            if let cta, !loading {
                Text(cta)
                    .font(.caption.weight(.semibold))
                    .foregroundStyle(RenderTheme.Colors.brandA.opacity(0.8))
            }
        }
        .frame(maxWidth: .infinity)
        .padding(.vertical, RenderTheme.Spacing.lg)
        .padding(.horizontal, RenderTheme.Spacing.md)
        .background(
            RoundedRectangle(cornerRadius: RenderTheme.Radius.card, style: .continuous)
                .strokeBorder(RenderTheme.Colors.hairline, style: StrokeStyle(lineWidth: 1, dash: [6, 5]))
        )
    }
}
