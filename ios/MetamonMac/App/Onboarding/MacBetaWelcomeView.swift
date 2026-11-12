// 審査用ビルドでは外す(β の文言を App Store の版に入れない・#313・docs/ios/review-build.md)。
#if !APP_STORE_REVIEW
import SwiftUI

/// β の案内(#313)の Mac 版(#971)。文言は iOS と同じ `BetaWelcomeContent`。
/// 依頼の例はクリックで会話のシートの入力欄に入れる(送らない・#1214)。案内からフィードバックのウインドウ(#983)も開ける。
struct MacBetaWelcomeView: View {
    let onFinish: () -> Void
    @Environment(\.openWindow) private var openWindow

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
                Section {
                    ForEach(BetaWelcomeContent.tryKeys, id: \.self) { key in
                        let text = L10n.tr(String.LocalizationValue(key))
                        Button {
                            MacDraftRequest.shared.send(text)
                            onFinish()
                        } label: {
                            Label(L10n.tr("「\(text)」"), systemImage: "mic")
                        }
                        .buttonStyle(.borderless)
                        .help(L10n.tr("クリックすると入力欄に入ります(まだ送りません)。"))
                        .accessibilityIdentifier("betaWelcome.example")
                    }
                } header: {
                    Text("試してほしい依頼")
                } footer: {
                    Text("クリックすると入力欄に入ります(まだ送りません)。")
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
                Section {
                    Button {
                        MacFeedbackCenter.shared.open(openWindow) // #983
                    } label: {
                        Label("フィードバックを送る", systemImage: "envelope")
                    }
                    .buttonStyle(.borderless)
                    .help(L10n.tr("フィードバックのウインドウを開きます"))
                    .accessibilityIdentifier("betaWelcome.feedback")
                } footer: {
                    Text("困ったこと・欲しい機能を、ここからそのまま送れます。")
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
        .frame(width: 520, height: 640)
        .accessibilityIdentifier("betaWelcome.sheet")
    }
}
#endif
