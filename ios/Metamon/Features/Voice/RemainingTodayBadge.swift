import SwiftUI
import MetamonServices

/// Pro かどうか。審査用の版は App 内課金を持たない(#450 G1)ので常に無料。
@MainActor
enum RemainingTodayPlan {
    static var isPro: Bool {
        #if APP_STORE_REVIEW
        false
        #else
        ProGate.shared.isPro
        #endif
    }
}

/// 音声のシートの入力欄の上に出す小さな「今日の残り N」。0 になったら上限のカードを 1 度だけ出す。
struct RemainingTodayBadge: View {
    @Environment(AppRuntime.self) private var rt
    @Environment(AuthSession.self) private var session

    /// 0 のカードを出した日(戻る時刻。分からなければ distantFuture = 起動中に 1 度)。閉じたら同じ日にはもう出さない
    @MainActor static var exhaustedCardShownFor: Date?

    private var value: RemainingToday? { RemainingTodayStore.shared.last }
    private var display: RemainingTodayDisplay {
        .of(value, now: .now, isPro: RemainingTodayPlan.isPro, isAnonymous: session.user?.isAnonymous ?? false)
    }

    var body: some View {
        content
            .task(id: display) { showExhaustedCardIfNeeded() }
    }

    @ViewBuilder
    private var content: some View {
        switch display {
        case .hidden:
            EmptyView()
        case .normal(let n, let pro), .low(let n, let pro):
            let low = display.isLow
            HStack(spacing: Theme.Spacing.xs) {
                if pro {
                    Text(verbatim: "Pro")
                        .font(.caption2.weight(.bold))
                        .foregroundStyle(.white)
                        .padding(.horizontal, 6)
                        .padding(.vertical, 1)
                        .background(Theme.brandGradient, in: Capsule())
                }
                if low { Image(systemName: "exclamationmark.circle.fill") }
                Text(RemainingTodayDisplay.label(n))
                    .monospacedDigit()
            }
            .font(.caption.weight(low ? .semibold : .regular))
            .foregroundStyle(low ? AnyShapeStyle(Theme.Colors.warning) : AnyShapeStyle(.secondary))
            .frame(maxWidth: .infinity, alignment: .trailing)
            .padding(.bottom, Theme.Spacing.xs)
            .accessibilityElement(children: .ignore)
            .accessibilityLabel(display.accessibilityLabel)
            .accessibilityHint(RemainingTodayDisplay.hint(resetAt: value?.resetAt))
            .accessibilityIdentifier("voice.remainingToday")
        case .exhausted:
            // 理由と戻る時刻は上限のカードが出す。ここは数だけ
            Text(RemainingTodayDisplay.label(0))
                .font(.caption.weight(.semibold))
                .foregroundStyle(Theme.Colors.warning)
                .monospacedDigit()
                .frame(maxWidth: .infinity, alignment: .trailing)
                .padding(.bottom, Theme.Spacing.xs)
                .accessibilityLabel(display.accessibilityLabel)
                .accessibilityHint(RemainingTodayDisplay.hint(resetAt: value?.resetAt))
                .accessibilityIdentifier("voice.remainingToday")
        }
    }

    private func showExhaustedCardIfNeeded() {
        guard case .exhausted(let info) = display, rt.errorCard == nil, !rt.isMorphing,
              Self.exhaustedCardShownFor != (info.resetAt ?? .distantFuture) else { return }
        Self.exhaustedCardShownFor = info.resetAt ?? .distantFuture
        withAnimation(.easeInOut(duration: 0.3)) { rt.errorCard = ErrorPresentation(kind: .rateLimited(info), transcript: "") }
    }
}

extension RemainingTodayDisplay {
    var isLow: Bool { if case .low = self { true } else { false } }
}
