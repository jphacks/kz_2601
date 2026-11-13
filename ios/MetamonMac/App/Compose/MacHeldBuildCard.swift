import SwiftUI
import MetamonServices

/// 1 日の枠が切れて作れなかった依頼のカード(#3079・iOS の `HeldBuildCard` の Mac 版)。
/// 会話はそのまま。枠が戻った・Pro にした後、「続きから作る」で同じ依頼から作る。見出しと本文は上限の状態カードと同じ(`ErrorPresentation`)。
struct MacHeldBuildCard: View {
    let presentation: ErrorPresentation
    var busy = false
    var onContinue: () -> Void
    var onDismiss: () -> Void

    var body: some View {
        HStack(alignment: .top, spacing: MacTheme.Spacing.group) {
            Image(systemName: "crown.fill")
                .font(.title3.weight(.semibold))
                .foregroundStyle(Theme.brandGradient)
                .frame(width: 24)
            VStack(alignment: .leading, spacing: 6) {
                Text(presentation.title).font(.headline)
                Text(presentation.body)
                    .font(.callout)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
                    .textSelection(.enabled)
                Text("会話はこのまま残っています。枠が戻るか Pro にすると、続きから同じ依頼で作れます。")
                    .font(.callout)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
                HStack(spacing: MacTheme.Spacing.group) {
                    Button("続きから作る", action: onContinue)
                        .buttonStyle(.borderedProminent)
                        .disabled(busy)
                        .accessibilityIdentifier("held.continue")
                    if MacErrorCard.showsAppleSignIn(presentation, isLive: MacAccount.shared.isLive) {
                        MacAppleSignInButton(relogin: false, onSignedIn: onContinue)
                    }
                    #if !APP_STORE_REVIEW
                    if presentation.primary == .upgradePro {
                        MacUpgradeProButton(reason: .planLimit)
                    }
                    #endif
                    Button(ErrorCopy.dismiss, action: onDismiss)
                        .accessibilityIdentifier("held.close")
                }
                .padding(.top, 2)
            }
            Spacer(minLength: 0)
        }
        .padding(MacTheme.Spacing.group)
        .background(.quaternary.opacity(0.5), in: RoundedRectangle(cornerRadius: 10, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: 10, style: .continuous).strokeBorder(.separator))
        .accessibilityElement(children: .contain)
        .accessibilityLabel(presentation.announcement)
    }
}
