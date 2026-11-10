import SwiftUI
import MetamonServices

/// 会話のシートの失敗を出す状態カード(#968)。分け方と文言は iOS の `ErrorPresentation` / `ErrorCopy` をそのまま使う
/// (上限・Pro が要る・一時停止・オフライン・混雑・失敗)。見た目は Mac に合わせ、ボタンは「もう一度」と「閉じる」。
/// サインイン・Pro にする・今すぐ送るは Mac にまだ無い(#930・課金は後の Issue)ので出さず、本文の案内だけにする。
struct MacErrorCard: View {
    let presentation: ErrorPresentation
    var retry: () -> Void
    var dismiss: () -> Void

    /// Mac で押せる主ボタン。もう一度で直るものだけ
    static func showsRetry(_ presentation: ErrorPresentation) -> Bool {
        switch presentation.primary {
        case .retry?: true
        case .sendNow?, .signIn?, .signInApple?, .upgradePro?, nil: false
        }
    }

    var body: some View {
        HStack(alignment: .top, spacing: MacTheme.Spacing.group) {
            Image(systemName: presentation.icon)
                .font(.title3.weight(.semibold))
                .foregroundStyle(presentation.isWarning ? AnyShapeStyle(Theme.Colors.warning) : AnyShapeStyle(Theme.brandGradient))
                .frame(width: 24)
            VStack(alignment: .leading, spacing: 6) {
                Text(presentation.title).font(.headline)
                Text(presentation.body)
                    .font(.callout)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
                    .textSelection(.enabled)
                HStack(spacing: MacTheme.Spacing.group) {
                    if Self.showsRetry(presentation) {
                        Button(ErrorCopy.retry, action: retry)
                            .buttonStyle(.borderedProminent)
                    }
                    Button(ErrorCopy.dismiss, action: dismiss)
                }
                .padding(.top, 2)
            }
            Spacer(minLength: 0)
            CraneMascot(pose: .panic, size: 44, sway: 3)
                .accessibilityHidden(true)
        }
        .padding(MacTheme.Spacing.group)
        .background(.quaternary.opacity(0.5), in: RoundedRectangle(cornerRadius: 10, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: 10, style: .continuous).strokeBorder(.separator))
        .accessibilityElement(children: .contain)
        .accessibilityLabel(presentation.announcement)
        .task(id: presentation.id) { MacVoiceOver.announce(presentation.announcement) }
    }
}
