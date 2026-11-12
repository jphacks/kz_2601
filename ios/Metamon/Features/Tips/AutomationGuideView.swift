import SwiftUI
import MetamonRenderer

/// 「ショートカットで自動にする」(#1995)。設定の「頼み方のコツ」の下から push で開く。
/// 例・手順・注意を見せ、ショートカット App を開くボタンを置く(オートメーション自体は利用者がそちらで作る)。
struct AutomationGuideView: View {
    @Environment(\.openURL) private var openURL
    @State private var openFailed = false

    var body: some View {
        List {
            Section {
                Text(AutomationGuide.intro)
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
                    .listRowBackground(Color.clear)
            }
            Section {
                ForEach(AutomationGuide.examples) { example in
                    HStack(alignment: .firstTextBaseline, spacing: Theme.Spacing.sm) {
                        Image(systemName: example.symbol)
                            .foregroundStyle(Theme.Colors.brandText)
                            .frame(width: 24)
                            .accessibilityHidden(true)
                        VStack(alignment: .leading, spacing: Theme.Spacing.xs) {
                            Text(example.trigger).font(.body.weight(.semibold))
                            Text(example.action)
                                .font(.subheadline)
                                .foregroundStyle(.secondary)
                                .fixedSize(horizontal: false, vertical: true)
                        }
                    }
                    .padding(.vertical, Theme.Spacing.xs)
                    .accessibilityElement(children: .combine)
                    .accessibilityIdentifier("automationGuide.example.\(example.id)")
                }
            } header: {
                Text(L10n.tr(String.LocalizationValue(AutomationGuide.examplesHeaderKey)))
            } footer: {
                Text(L10n.tr(String.LocalizationValue(AutomationGuide.otherActionsKey)))
            }
            Section {
                ForEach(Array(AutomationGuide.steps.enumerated()), id: \.offset) { index, step in
                    HStack(alignment: .firstTextBaseline, spacing: Theme.Spacing.sm) {
                        Text(verbatim: "\(index + 1).")
                            .font(.body.monospacedDigit().weight(.semibold))
                            .foregroundStyle(Theme.Colors.brandText)
                        Text(step).fixedSize(horizontal: false, vertical: true)
                    }
                    .accessibilityElement(children: .combine)
                }
                Button {
                    openURL(AutomationGuide.shortcutsURL) { accepted in openFailed = !accepted }
                } label: {
                    Label(L10n.tr(String.LocalizationValue(AutomationGuide.openButtonKey)), systemImage: "arrow.up.forward.app")
                }
                .accessibilityIdentifier("automationGuide.openShortcuts")
            } header: {
                Text(L10n.tr(String.LocalizationValue(AutomationGuide.stepsHeaderKey)))
            } footer: {
                VStack(alignment: .leading, spacing: Theme.Spacing.xs) {
                    if openFailed {
                        Text(L10n.tr(String.LocalizationValue(AutomationGuide.openFailedKey)))
                            .foregroundStyle(.red)
                            .accessibilityIdentifier("automationGuide.openFailed")
                    }
                    ForEach(AutomationGuide.notes, id: \.self) { Text($0) }
                }
            }
        }
        .navigationTitle(AutomationGuide.title)
        .navigationBarTitleDisplayMode(.inline)
        .accessibilityIdentifier("automationGuide.list")
    }
}
