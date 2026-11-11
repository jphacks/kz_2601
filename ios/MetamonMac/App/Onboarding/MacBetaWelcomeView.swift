// 審査用ビルドでは外す(β の文言を App Store の版に入れない・#313・docs/ios/review-build.md)。
#if !APP_STORE_REVIEW
import SwiftUI

/// β の案内(#313)の Mac 版(#971)。文言は iOS と同じ `BetaWelcomeContent`。
/// Mac にはまだ入力欄(#967)とフィードバックの送信(#319)が無いので、依頼の例は読むだけ・フィードバックの欄は出さない。
struct MacBetaWelcomeView: View {
    let onFinish: () -> Void

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    HStack(alignment: .top, spacing: Theme.Spacing.md) {
                        CraneMascot(pose: .happy, size: 64, sway: 4)
                            .accessibilityHidden(true)
                        VStack(alignment: .leading, spacing: Theme.Spacing.xs) {
                            Text("Tailor It β へようこそ")
                                .font(.headline)
                                .accessibilityAddTraits(.isHeader)
                            Text("β 版を試していただき、ありがとうございます。使ってみて気づいたことを、ぜひ教えてください。")
                                .fixedSize(horizontal: false, vertical: true)
                        }
                    }
                }
                Section("試してほしい依頼") {
                    ForEach(BetaWelcomeContent.tryKeys, id: \.self) { key in
                        let text = L10n.tr(String.LocalizationValue(key))
                        Label(L10n.tr("「\(text)」"), systemImage: "mic")
                    }
                }
                Section("まだできないこと") {
                    ForEach(BetaWelcomeContent.limitKeys, id: \.self) { key in
                        Label(L10n.tr(String.LocalizationValue(key)), systemImage: "exclamationmark.circle")
                    }
                }
                Section("データについて") {
                    Label("β の期間中は、作ったアプリや記録が消えることがあります。大事なデータは、ほかの場所にも残してください。",
                          systemImage: "externaldrive.badge.exclamationmark")
                }
            }
            .formStyle(.grouped)
            .navigationTitle("β 版のご案内")
            .toolbar {
                ToolbarItem(placement: .confirmationAction) {
                    Button("はじめる") { onFinish() }
                        .keyboardShortcut(.defaultAction)
                        .accessibilityIdentifier("betaWelcome.close")
                }
            }
        }
        .frame(width: 520, height: 600)
        .accessibilityIdentifier("betaWelcome.sheet")
    }
}
#endif
