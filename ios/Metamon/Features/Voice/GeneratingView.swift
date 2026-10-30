import SwiftUI
import MetamonDSL

/// 生成中の経過段階(docs/backend/ai-eval 2026-10-24 §5 と 01 §4 の予算に対応)。
/// - 0〜10s  normal   : create p50 6.6s / p95 8.7s。数秒ごとに文言を回す
/// - 10〜20s slow     : 「もう少しかかっています」(area 再生成 1 回 ≒ +7s)
/// - 20〜30s warning  : 警告色。backend は 20s で警告ライン
/// - 30s〜   fallback : backend がテンプレ DSL にフォールバック中。キャンセルを出す(HTTP は 45s で timeout)
enum GeneratingStage: Equatable {
    case normal, slow, warning, fallback

    init(elapsed: TimeInterval) {
        switch elapsed {
        case ..<10: self = .normal
        case ..<20: self = .slow
        case ..<30: self = .warning
        default: self = .fallback
        }
    }

    /// 文言表(docs/ios/architecture.md §6 と同期)
    static var normalMessages: [String] { [
        L10n.pick(L10n.tr("どんなアプリにするか考えています…"), review: L10n.tr("どんな記録帳にするか考えています…")),
        L10n.tr("必要な画面とデータを決めています…"),
        L10n.tr("画面を組み立てています…"),
        L10n.tr("細部を整えています…"),
    ] }

    func message(elapsed: TimeInterval, edit: Bool) -> String {
        switch self {
        case .normal:
            if edit { return L10n.tr("変更を反映しています…") }
            let i = Int(elapsed / 3) % Self.normalMessages.count
            return Self.normalMessages[i]
        case .slow: return L10n.tr("もう少しかかっています…")
        case .warning: return L10n.tr("時間がかかっています。もう少しだけお待ちください")
        case .fallback: return L10n.tr("混み合っています。安全なテンプレートに切り替えています…")
        }
    }
}

/// create(初期状態からの生成)の全画面「変身中」演出。
/// 見出し = 認識テキスト、段階文言、6×12 のスケルトングリッド(セルが順に光り、数秒ごとにレイアウト案が入れ替わる)。
struct GeneratingView: View {
    @Environment(AppRuntime.self) private var rt
    @Environment(\.colorScheme) private var scheme
    let job: AppRuntime.MorphJob

    @State private var elapsed: TimeInterval = 0

    private var stage: GeneratingStage { GeneratingStage(elapsed: elapsed) }

    var body: some View {
        ZStack {
            Theme.backdrop(scheme)
            Rectangle().fill(.ultraThinMaterial).ignoresSafeArea()

            VStack(spacing: Theme.Spacing.lg) {
                header
                SkeletonGrid(elapsed: elapsed)
                    .frame(maxWidth: 240)
                    .aspectRatio(6.0 / 9.0, contentMode: .fit)
                    .padding(.horizontal, Theme.Spacing.lg)
                footer
            }
            .padding(.vertical, Theme.Spacing.xl)
            .padding(.horizontal, Theme.Spacing.lg)
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("generating.create") // UI テスト(#182)
        #if DEBUG
        .onAppear { MorphProbe.generatingAppeared += 1 }
        #endif
        .task(id: job.startedAt) {
            // 0.25s 刻みで経過秒を更新(文言切替と段階判定用。グリッドの描画は TimelineView が担う)
            while !Task.isCancelled {
                elapsed = Date.now.timeIntervalSince(job.startedAt)
                try? await Task.sleep(for: .milliseconds(250))
            }
        }
    }

    private var header: some View {
        VStack(spacing: Theme.Spacing.sm) {
            // #102: 作業中のツル(PC とスマホを数秒ごとに持ち替える)
            CraneMascot(pose: Int(elapsed / 4) % 2 == 0 ? .pc : .phone, size: 96, sway: 3)
            Label(L10n.key("変身中", review: "準備中"), systemImage: "sparkles")
                .font(.caption.weight(.semibold))
                .textCase(.uppercase)
                .tracking(1.2)
                .foregroundStyle(Theme.brandGradient)
            Text("「\(displayTranscript)」")
                .font(.title3.weight(.semibold))
                .multilineTextAlignment(.center)
                .lineLimit(3)
                .minimumScaleFactor(0.8)
        }
    }

    /// 見出しには「別のアプリ。」などの切替キーワードを出さない(VoiceIntent.newAppPrefixes と同じ語)。
    private var displayTranscript: String {
        let prefixes = ["別のアプリ", "別アプリ", "新しいアプリ", "新規アプリ", "違うアプリ", "次のアプリ", "べつのアプリ", "あたらしいアプリ"]
        guard let p = prefixes.first(where: { job.transcript.hasPrefix($0) }) else { return job.transcript }
        let rest = job.transcript.dropFirst(p.count).trimmingCharacters(in: .whitespacesAndNewlines.union(.punctuationCharacters))
        return rest.isEmpty ? job.transcript : rest
    }

    private var footer: some View {
        VStack(spacing: Theme.Spacing.md) {
            HStack(spacing: Theme.Spacing.sm) {
                ProgressView().tint(stage == .warning || stage == .fallback ? Theme.Colors.warning : nil)
                Text(stage.message(elapsed: elapsed, edit: false))
                    .font(.subheadline.weight(.medium))
                    .foregroundStyle(stage == .warning || stage == .fallback ? Theme.Colors.warning : .secondary)
                    .contentTransition(.opacity)
            }
            .animation(.easeInOut(duration: 0.3), value: stage.message(elapsed: elapsed, edit: false))

            Text(elapsedLabel)
                .font(.caption2.monospacedDigit())
                .foregroundStyle(.tertiary)

            if stage == .fallback {
                Button(role: .cancel) { rt.cancelMorph() } label: {
                    Text("キャンセル").font(.subheadline.weight(.semibold))
                }
                .buttonStyle(.bordered)
                .transition(.opacity.combined(with: .scale(scale: 0.9)))
            }
        }
        .animation(.easeInOut(duration: 0.3), value: stage)
    }

    private var elapsedLabel: String {
        L10n.tr("\(Int(elapsed.rounded(.down))) 秒")
    }
}

#if DEBUG
/// UI テストの観測用カウンタ(RootView の `UITestMorphProbe` が読む・#200)
@MainActor
enum MorphProbe {
    static var createStarted = 0
    static var generatingAppeared = 0
    /// RootView の body が create の生成中で評価された回数(0 なら RootView が morphJob の変化で作り直されていない)
    static var rootCreateEvals = 0
    /// VoiceInputView 側の保険(`GeneratingFallback`)が出た回数
    static var fallbackAppeared = 0
}
#endif

/// 6×12 のスケルトングリッド。セルが波のように順に光り、3 秒ごとに「レイアウト案」(ブロック)が入れ替わる。
/// TimelineView(20fps) + 単純な sin で軽量に。
struct SkeletonGrid: View {
    let elapsed: TimeInterval

    private static let cols = GridArea.columns
    private static let rows = GridArea.visibleRows
    /// レイアウト案(area 文字列)。実 DSL と同じ記法。
    private static let layouts: [[String]] = [
        ["A1:F3", "A4:C7", "D4:F7", "A8:F12"],
        ["A1:F2", "A3:F8", "A9:C12", "D9:F12"],
        ["A1:B4", "C1:F4", "A5:F9", "A10:F12"],
        ["A1:F4", "A5:B12", "C5:F8", "C9:F12"],
    ]

    private var layoutIndex: Int { Int(elapsed / 3) % Self.layouts.count }

    /// n セル分の長さ(間の gap 込み)。0 未満にしない
    private static func span(_ n: Int, cell: CGFloat, gap: CGFloat) -> CGFloat {
        max(0, CGFloat(n) * (cell + gap) - gap)
    }

    var body: some View {
        GeometryReader { geo in
            let gap: CGFloat = 6
            // 出し入れの途中は geo.size が 0 になり得る。負の幅は "Invalid frame dimension" になるので 0 で止める
            let cw = max(0, (geo.size.width - gap * CGFloat(Self.cols - 1)) / CGFloat(Self.cols))
            let ch = max(0, (geo.size.height - gap * CGFloat(Self.rows - 1)) / CGFloat(Self.rows))
            TimelineView(.periodic(from: .now, by: 1.0 / 20)) { tl in
                let t = tl.date.timeIntervalSinceReferenceDate
                ZStack(alignment: .topLeading) {
                    // ドット(全セル): 斜めに走る光
                    ForEach(0..<(Self.cols * Self.rows), id: \.self) { i in
                        let c = i % Self.cols, r = i / Self.cols
                        let phase = t * 2.2 - Double(c + r) * 0.35
                        let glow = max(0, sin(phase))
                        RoundedRectangle(cornerRadius: 3, style: .continuous)
                            .fill(Theme.Colors.brandA.opacity(0.05 + 0.28 * glow * glow))
                            .frame(width: cw, height: ch)
                            .offset(x: CGFloat(c) * (cw + gap), y: CGFloat(r) * (ch + gap))
                    }
                    // ブロック(レイアウト案)
                    ForEach(Self.layouts[layoutIndex], id: \.self) { a in
                        if let area = GridArea(a) {
                            RoundedRectangle(cornerRadius: Theme.Radius.sm, style: .continuous)
                                .fill(Theme.brandGradient.opacity(0.14))
                                .overlay(RoundedRectangle(cornerRadius: Theme.Radius.sm, style: .continuous)
                                    .strokeBorder(Theme.Colors.brandB.opacity(0.5), lineWidth: 1))
                                .frame(width: Self.span(area.colSpan, cell: cw, gap: gap),
                                       height: Self.span(area.rowSpan, cell: ch, gap: gap))
                                .offset(x: CGFloat(area.col1) * (cw + gap), y: CGFloat(area.row1 - 1) * (ch + gap))
                                .transition(.opacity)
                        }
                    }
                }
                .animation(.easeInOut(duration: 0.35), value: layoutIndex)
            }
        }
    }
}

/// edit(2 回目以降)の軽いインジケータ。現在の画面は RootView 側で薄くし、上部に小さなカプセルを出す。
struct EditingIndicator: View {
    @Environment(AppRuntime.self) private var rt
    let job: AppRuntime.MorphJob
    @State private var elapsed: TimeInterval = 0

    private var stage: GeneratingStage { GeneratingStage(elapsed: elapsed) }
    private var warn: Bool { stage == .warning || stage == .fallback }

    var body: some View {
        VStack(spacing: Theme.Spacing.sm) {
            HStack(spacing: Theme.Spacing.sm) {
                ProgressView().controlSize(.small).tint(warn ? Theme.Colors.warning : nil)
                VStack(alignment: .leading, spacing: 2) {
                    Text(stage.message(elapsed: elapsed, edit: true))
                        .font(.footnote.weight(.semibold))
                        .foregroundStyle(warn ? Theme.Colors.warning : .primary)
                    Text("「\(job.transcript)」")
                        .font(.caption2)
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                }
                if stage == .fallback {
                    Button("中止") { rt.cancelMorph() }
                        .font(.caption.weight(.semibold))
                        .buttonStyle(.bordered)
                        .controlSize(.mini)
                }
            }
            .padding(.horizontal, Theme.Spacing.md)
            .padding(.vertical, Theme.Spacing.sm + 2)
            .background(.regularMaterial, in: Capsule())
            .overlay(Capsule().strokeBorder(Theme.Colors.hairline))
            .shadow(color: Theme.Shadow.softColor, radius: 12, y: 6)
            .accessibilityElement(children: .contain)
            .accessibilityIdentifier("generating.edit") // UI テスト(#182)
        }
        .animation(.easeInOut(duration: 0.3), value: stage)
        .task(id: job.startedAt) {
            while !Task.isCancelled {
                elapsed = Date.now.timeIntervalSince(job.startedAt)
                try? await Task.sleep(for: .milliseconds(250))
            }
        }
    }
}
