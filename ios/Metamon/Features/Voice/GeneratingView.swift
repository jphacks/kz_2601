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
/// 見出し = 認識テキスト、段階文言、組み立て台(#626: パーツ箱から部品を選んで空の画面に置く・入れ替える・並べ直す)。
/// 結果が届いたら(`reveal`)本物の部品を上から置いて組み上げ、RootView が本物の画面に切り替える。
struct GeneratingView: View {
    @Environment(AppRuntime.self) private var rt
    @Environment(\.colorScheme) private var scheme
    let job: AppRuntime.MorphJob
    /// 結果の組み上げ中(#626)。nil なら待っている間
    var reveal: BuildReveal? = nil

    @State private var elapsed: TimeInterval = 0

    private var stage: GeneratingStage { GeneratingStage(elapsed: elapsed) }
    /// 待っている間の台本(依頼文から部品を当てる)
    private var script: BuildChoreography.WaitingScript { BuildChoreography.WaitingScript(transcript: job.transcript) }
    /// 段階文言。組み上げ中は「画面を組み立てています…」
    private var message: String { reveal != nil ? L10n.tr("画面を組み立てています…") : stage.message(elapsed: elapsed, edit: false) }
    private var warn: Bool { reveal == nil && (stage == .warning || stage == .fallback) }

    var body: some View {
        ZStack {
            Theme.backdrop(scheme)
            Rectangle().fill(.ultraThinMaterial).ignoresSafeArea()

            VStack(spacing: Theme.Spacing.lg) {
                header
                BuildStageView(script: script, elapsed: elapsed, plan: reveal?.plan, live: job.draft)
                    .padding(.horizontal, Theme.Spacing.lg)
                footer
            }
            .padding(.vertical, Theme.Spacing.xl)
            .padding(.horizontal, Theme.Spacing.lg)
        }
        // 組み上げ中は触ると飛ばして本物の画面へ(待たせない)
        .contentShape(Rectangle())
        .onTapGesture { if reveal != nil { rt.finishBuildReveal() } }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("generating.create") // UI テスト(#182)。組み上げ中も「作成中」のまま
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
            // SSE で届いたアプリ名(#636・仮置き)
            if let name = job.draft.appName {
                Text(name)
                    .font(.headline)
                    .foregroundStyle(Theme.brandGradient)
                    .lineLimit(1)
                    .transition(.opacity)
            }
            Text("「\(displayTranscript)」")
                .font(.title3.weight(.semibold))
                .multilineTextAlignment(.center)
                .lineLimit(3)
                .minimumScaleFactor(0.8)
        }
        .animation(.easeInOut(duration: 0.3), value: job.draft.appName)
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
                ProgressView().tint(warn ? Theme.Colors.warning : nil)
                Text(message)
                    .font(.subheadline.weight(.medium))
                    .foregroundStyle(warn ? Theme.Colors.warning : .secondary)
                    .contentTransition(.opacity)
            }
            .animation(.easeInOut(duration: 0.3), value: message)

            Text(elapsedLabel)
                .font(.caption2.monospacedDigit())
                .foregroundStyle(.tertiary)

            if stage == .fallback, reveal == nil {
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
                if warn {
                    ProgressView().controlSize(.small).tint(Theme.Colors.warning)
                } else {
                    EditShuffleGlyph(elapsed: elapsed) // #626 部品を並べ直している感じ
                }
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
