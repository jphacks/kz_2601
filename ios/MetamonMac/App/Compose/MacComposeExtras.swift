import SwiftUI
import MetamonServices

/// 会話のシートのあいさつ(#1211)。見出しは iOS と同じ `Greeting`(時間帯・初回 / 再訪)、
/// 下の一言は Mac の入力(文字で書く)に合わせる(iOS の「マイクを長押し」は Mac に無いため)。
enum MacComposeGreeting {
    static func copy(for target: MacComposeSession.Target, returning: Bool = Greeting.launchIsReturning,
                     date: Date = .now, calendar: Calendar = .current) -> Greeting.Copy {
        let hello = Greeting.timeOfDay(date, calendar: calendar)
        switch target {
        case .create:
            let title = Greeting.copy(for: returning ? .returning : .firstVisit, date: date, calendar: calendar).title
            return .init(title: title,
                         subtitle: L10n.tr("\(hello)。「毎日の支出を記録して、今月の合計を見たい」のように、使い道を書いてください。"))
        case .edit:
            return .init(title: Greeting.copy(for: .editing, date: date, calendar: calendar).title,
                         subtitle: L10n.tr("\(hello)。「合計をもっと大きく表示して」のように、変えたい所を書いてください。"))
        }
    }
}

/// シートの上の「今日の残り N」(#1211・iOS の `RemainingTodayBadge`)。分からない・古い時は出さない。
/// 0 の時は数字だけ出す(上限の状態カードは送った時の失敗で出る・#968)
struct MacRemainingTodayBadge: View {
    var store: RemainingTodayStore = .shared

    private var display: RemainingTodayDisplay {
        .of(store.last, now: .now, isPro: RemainingTodayPlan.isPro,
            isAnonymous: MacAccount.shared.session.user?.isAnonymous ?? false)
    }

    var body: some View {
        if let text = Self.text(display) {
            let warn = Self.isWarning(display)
            HStack(spacing: 4) {
                if Self.isPro(display) {
                    Text(verbatim: "Pro")
                        .font(.caption2.weight(.bold))
                        .foregroundStyle(.white)
                        .padding(.horizontal, 6)
                        .padding(.vertical, 1)
                        .background(Theme.brandGradient, in: Capsule())
                }
                if warn { Image(systemName: "exclamationmark.circle.fill") }
                Text(text).monospacedDigit()
            }
            .font(.caption.weight(warn ? .semibold : .regular))
            .foregroundStyle(warn ? AnyShapeStyle(Theme.Colors.warning) : AnyShapeStyle(.secondary))
            .help(RemainingTodayDisplay.hint(resetAt: store.last?.resetAt))
            .accessibilityElement(children: .ignore)
            .accessibilityLabel(display.accessibilityLabel)
            .accessibilityHint(RemainingTodayDisplay.hint(resetAt: store.last?.resetAt))
            .accessibilityIdentifier("compose.remainingToday")
        }
    }

    /// 出す文言。分からない時は nil(出さない)
    static func text(_ display: RemainingTodayDisplay) -> String? {
        switch display {
        case .hidden: nil
        case .normal(let n, _), .low(let n, _): RemainingTodayDisplay.label(n)
        case .exhausted: RemainingTodayDisplay.label(0)
        }
    }

    static func isWarning(_ display: RemainingTodayDisplay) -> Bool {
        switch display {
        case .low, .exhausted: true
        case .hidden, .normal: false
        }
    }

    static func isPro(_ display: RemainingTodayDisplay) -> Bool {
        switch display {
        case .normal(_, let pro), .low(_, let pro): pro
        case .hidden, .exhausted: false
        }
    }
}

/// 最新の質問の下の「質問 n / 3」(#1211・iOS の `QuestionProgressLabel`)
struct MacQuestionProgressLabel: View {
    let number: Int

    var body: some View {
        Text(QuestionProgress.label(number))
            .font(.caption.weight(.medium))
            .foregroundStyle(.secondary)
            .monospacedDigit()
            .padding(.leading, 4)
            .accessibilityIdentifier("compose.questionProgress")
    }
}

/// 閉じた会話の 1 件(#1211)。吹き出しは今の会話と同じ見た目、反映は区切り線 +「ここで反映しました(v3)・〜」
struct MacHistoryRow<Bubble: View>: View {
    let entry: ChatHistoryEntry
    @ViewBuilder var bubble: (ChatMessage.Role, String) -> Bubble

    var body: some View {
        switch entry.kind {
        case .user: bubble(.user, entry.text)
        case .assistant: bubble(.assistant, entry.text)
        case .applied:
            HStack(spacing: 8) {
                line
                Label(ChatContext.dividerText(entry), systemImage: "checkmark.circle.fill")
                    .font(.caption.weight(.medium))
                    .foregroundStyle(.secondary)
                    .multilineTextAlignment(.center)
                    .fixedSize(horizontal: false, vertical: true)
                line
            }
            .padding(.vertical, 4)
            .accessibilityElement(children: .combine)
            .accessibilityIdentifier("compose.history.applied")
        }
    }

    private var line: some View {
        Rectangle().fill(.separator).frame(height: 1).frame(maxWidth: .infinity)
    }
}
