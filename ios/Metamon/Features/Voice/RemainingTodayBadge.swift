import SwiftUI
import MetamonServices

/// 「今日の残り N」(#630 M1・X3)の出し分け。単位はサーバーの重み(新規 3・修正 2・チャット 1)なので「回」と言わない。
enum RemainingTodayDisplay: Equatable {
    /// まだ分からない・日付が変わって古い
    case hidden
    case normal(Int, pro: Bool)
    /// 少ない(`lowThreshold` 以下)。目立たせる
    case low(Int, pro: Bool)
    /// 0。既存の上限のカード(#280)に渡す
    case exhausted(RateLimitInfo)

    /// 新しく作るのがあと 2 回分を切ったら少ない扱い
    static let lowThreshold = RemainingToday.Weight.create * 2

    static func of(_ value: RemainingToday?, now: Date, isPro: Bool, isAnonymous: Bool) -> RemainingTodayDisplay {
        guard let value, !value.isStale(now: now) else { return .hidden }
        if value.remaining <= 0 {
            let tier = isPro ? "pro" : (isAnonymous ? "anonymous" : "signed_in")
            return .exhausted(RateLimitInfo(scope: .user, tier: tier, limit: value.limit, remaining: 0, resetAt: value.resetAt))
        }
        return value.remaining <= lowThreshold ? .low(value.remaining, pro: isPro) : .normal(value.remaining, pro: isPro)
    }

    static func label(_ n: Int) -> String { L10n.tr("今日の残り \(n)") }

    /// VoiceOver で読む 1 文
    var accessibilityLabel: String {
        switch self {
        case .hidden: ""
        case .normal(let n, let pro): (pro ? L10n.tr("Pro。") : "") + Self.label(n)
        case .low(let n, let pro): (pro ? L10n.tr("Pro。") : "") + Self.label(n) + L10n.tr("。少なくなっています")
        case .exhausted: Self.label(0)
        }
    }

    /// 何で減るか・いつ戻るか
    static func hint(resetAt: Date?) -> String {
        let when = resetAt.map { ErrorCopy.resetTime($0) } ?? L10n.tr("明日")
        let c = RemainingToday.Weight.create, e = RemainingToday.Weight.edit, t = RemainingToday.Weight.chat
        return L10n.pick(L10n.tr("新しく作ると \(c)、直すと \(e)、相談すると \(t) 減ります。\(when)に戻ります"),
                         review: L10n.tr("新しく用意すると \(c)、直すと \(e)、相談すると \(t) 減ります。\(when)に戻ります"))
    }
}

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
