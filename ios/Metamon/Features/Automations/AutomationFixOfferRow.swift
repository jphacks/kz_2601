// 審査用ビルド(APP_STORE_REVIEW)では外す: 審査用の版は AI が serverRules を作らない(#614)。
#if !APP_STORE_REVIEW
import SwiftUI
import MetamonServices

// 規則・保存・モデル(`AutomationFixOffer`・`AutomationFixOfferStore`・`AutomationFixOfferModel`)は Shared/Automations に置く(Mac と共有)。

/// チャットの入力欄の上に出す 1 行。「直す」は依頼を入力欄に入れる(送らない)。
struct AutomationFixOfferRow: View {
    @Environment(AppRuntime.self) private var rt
    let model: AutomationFixOfferModel

    var body: some View {
        if let rule = model.rule {
            HStack(alignment: .center, spacing: Theme.Spacing.sm) {
                Image(systemName: "wrench.and.screwdriver")
                    .foregroundStyle(Theme.brandGradient)
                Text(verbatim: AutomationFixOffer.message(for: rule))
                    .font(.subheadline)
                    .fixedSize(horizontal: false, vertical: true)
                    .frame(maxWidth: .infinity, alignment: .leading)
                Button {
                    if let text = model.fix() { rt.composerDraft = text }
                } label: {
                    Text("直す")
                        .font(.subheadline.weight(.semibold))
                        .foregroundStyle(.white)
                        .padding(.horizontal, Theme.Spacing.md)
                        .padding(.vertical, Theme.Spacing.xs + 2)
                        .background(Theme.brandGradient, in: Capsule())
                }
                .buttonStyle(.plain)
                .accessibilityHint(Text("直す依頼を入力欄に入れます(まだ送りません)"))
                .accessibilityIdentifier("chat.automationFix.fix")
                Button { model.dismiss() } label: {
                    Image(systemName: "xmark.circle.fill").foregroundStyle(.secondary)
                }
                .buttonStyle(.plain)
                .accessibilityLabel(Text("閉じる"))
                .accessibilityIdentifier("chat.automationFix.dismiss")
            }
            .padding(.horizontal, Theme.Spacing.md)
            .padding(.vertical, Theme.Spacing.sm)
            .background(Theme.Colors.surface, in: RoundedRectangle(cornerRadius: 16, style: .continuous))
            .overlay(RoundedRectangle(cornerRadius: 16, style: .continuous).strokeBorder(Theme.Colors.brandA.opacity(0.35)))
            .transition(.opacity)
            .accessibilityElement(children: .contain)
            .accessibilityIdentifier("chat.automationFix")
        }
    }
}
#endif
