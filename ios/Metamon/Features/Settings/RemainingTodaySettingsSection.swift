import SwiftUI
import MetamonServices

/// 設定の「今日の残り」(#630 M1)。最後に分かった残り・上限・戻る時刻。AI を使うまでは分からない。
struct RemainingTodaySettingsSection: View {
    var body: some View {
        let value = RemainingTodayStore.shared.current()
        let pro = RemainingTodayPlan.isPro
        Section {
            LabeledContent(L10n.tr("今日の残り")) {
                HStack(spacing: Theme.Spacing.xs) {
                    if pro, value != nil { Text(verbatim: "Pro").fontWeight(.semibold).foregroundStyle(Theme.Colors.brandText) }
                    Text(Self.amount(value))
                        .monospacedDigit()
                        .foregroundStyle(Self.isLow(value) ? AnyShapeStyle(Theme.Colors.warning) : AnyShapeStyle(.secondary))
                }
            }
            .accessibilityElement(children: .combine)
            .accessibilityIdentifier("settings.remainingToday")
            if let reset = value?.resetAt {
                LabeledContent(L10n.tr("戻る時刻"), value: ErrorCopy.resetTime(reset))
                    .accessibilityIdentifier("settings.remainingToday.reset")
            }
        } footer: {
            Text(RemainingTodayDisplay.hint(resetAt: value?.resetAt))
        }
    }

    /// 「24 / 60」(上限が分からなければ数だけ)。古い・まだ分からない時は「AI を使うと表示されます」
    static func amount(_ value: RemainingToday?) -> String {
        guard let value else { return L10n.tr("AI を使うと表示されます") }
        guard let limit = value.limit else { return "\(value.remaining)" }
        return "\(value.remaining) / \(limit)"
    }

    static func isLow(_ value: RemainingToday?) -> Bool {
        guard let value else { return false }
        return value.remaining <= RemainingTodayDisplay.lowThreshold
    }
}
