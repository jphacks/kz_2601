import SwiftUI
import MetamonServices

/// 状態カードの見た目。`Theme` のカード(角丸・影)に合わせ、主ボタンはブランド色、副ボタンは文字だけ。
struct ErrorCard: View {
    @Environment(AppRuntime.self) private var rt
    let presentation: ErrorPresentation
    /// 生成後の画面上部(コンパクト)か、音声画面の中(通常)か
    var compact = false

    var body: some View {
        HStack(alignment: .top, spacing: Theme.Spacing.sm + 2) {
            Image(systemName: presentation.icon)
                .font(.title3.weight(.semibold))
                .foregroundStyle(presentation.isWarning ? AnyShapeStyle(Theme.Colors.warning) : AnyShapeStyle(Theme.brandGradient))
                .frame(width: 28)
                .padding(.top, 1)
            VStack(alignment: .leading, spacing: Theme.Spacing.xs + 2) {
                Text(presentation.title)
                    .font(.subheadline.weight(.semibold))
                Text(presentation.body)
                    .font(.footnote)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
                if !presentation.transcript.isEmpty, case .offlineQueued = presentation.kind {
                    Text("「\(presentation.transcript)」")
                        .font(.caption)
                        .foregroundStyle(.tertiary)
                        .lineLimit(1)
                }
                HStack(spacing: Theme.Spacing.md) {
                    if let primary = presentation.primary {
                        Button { rt.handle(primary, of: presentation) } label: {
                            Text(label(primary))
                                .font(.footnote.weight(.semibold))
                                .foregroundStyle(.white)
                                .padding(.horizontal, Theme.Spacing.md)
                                .padding(.vertical, Theme.Spacing.sm)
                                .background(Theme.brandGradient, in: Capsule())
                        }
                        .buttonStyle(.plain)
                        .accessibilityIdentifier("error.primary")
                    }
                    Button { rt.handle(presentation.secondary, of: presentation) } label: {
                        Text(label(presentation.secondary))
                            .font(.footnote.weight(.medium))
                            .foregroundStyle(.secondary)
                            .padding(.vertical, Theme.Spacing.sm)
                    }
                    .buttonStyle(.plain)
                    .accessibilityIdentifier("error.secondary")
                }
                .padding(.top, Theme.Spacing.xs)
            }
            Spacer(minLength: 0)
            CraneMascot(pose: .panic, size: compact ? 52 : 68, sway: 3) // #102
        }
        .metamonCard(padding: compact ? Theme.Spacing.md : Theme.Spacing.md + Theme.Spacing.xs)
        .accessibilityElement(children: .contain)
        .accessibilityLabel(presentation.announcement)
    }

    private func label(_ p: ErrorPresentation.Primary) -> String {
        switch p {
        case .retry: ErrorCopy.retry
        case .sendNow: ErrorCopy.sendNow
        case .signIn: ErrorCopy.signIn
        case .signInApple: ErrorCopy.signInApple
        case .upgradePro: ErrorCopy.upgradePro
        }
    }

    private func label(_ s: ErrorPresentation.Secondary) -> String {
        switch s {
        case .discard: ErrorCopy.discard
        case .dismiss: ErrorCopy.dismiss
        }
    }
}
