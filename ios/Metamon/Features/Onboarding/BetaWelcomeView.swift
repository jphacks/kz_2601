// 審査用ビルドでは外す(β の文言を App Store の版に入れない・#313・docs/ios/review-build.md)。
#if !APP_STORE_REVIEW
import SwiftUI
import MetamonServices

/// RootView に付ける: β の版でだけ、最初のアプリの後に 1 回出す(H1)。
struct BetaWelcomeHost: ViewModifier {
    @Environment(AppRuntime.self) private var rt

    func body(content: Content) -> some View {
        content
            .sheet(isPresented: Binding(get: { rt.showBetaWelcome },
                                        set: { if !$0 { rt.finishBetaWelcome() } })) {
                BetaWelcomeView()
            }
            .onChange(of: rt.showFirstEditHint) { wasShowing, showing in
                // 最初のアプリの直後の「直し方の例」が閉じたら出す(H1: 初回の関門から外し、最初のアプリの後へ)
                guard wasShowing, !showing else { return }
                Task { @MainActor in
                    try? await Task.sleep(for: .milliseconds(700))
                    rt.presentBetaWelcomeIfNeeded()
                }
            }
            .task {
                // 前の起動で最初のアプリを作った人(直し方の例を閉じる前に終了した・β に入れ直した等)は、起動して少し待ってから出す
                try? await Task.sleep(for: .milliseconds(1200))
                rt.presentBetaWelcomeIfNeeded()
            }
    }
}

/// β の案内(#313): 試してほしい依頼・まだできないこと・データが消える可能性・フィードバック(#319)への入口。
struct BetaWelcomeView: View {
    @Environment(AppRuntime.self) private var rt
    @Environment(\.services) private var services
    @State private var feedback: FeedbackContext?

    var body: some View {
        NavigationStack {
            List {
                Section {
                    HStack(alignment: .top, spacing: Theme.Spacing.md) {
                        CraneMascot(pose: .happy, size: 64, sway: 4)
                            .accessibilityHidden(true)
                        VStack(alignment: .leading, spacing: Theme.Spacing.xs) {
                            Text("Tailor It β へようこそ")
                                .font(.headline)
                                .accessibilityAddTraits(.isHeader)
                            Text("β 版を試していただき、ありがとうございます。使ってみて気づいたことを、ぜひ教えてください。")
                                .font(.subheadline)
                                .fixedSize(horizontal: false, vertical: true)
                        }
                    }
                }

                Section {
                    ForEach(BetaWelcomeContent.tryKeys, id: \.self) { key in
                        let text = L10n.tr(String.LocalizationValue(key))
                        Button { rt.useBetaWelcomeExample(text) } label: {
                            Label(L10n.tr("「\(text)」"), systemImage: "mic")
                        }
                    }
                } header: {
                    Text("試してほしい依頼")
                } footer: {
                    Text("タップすると入力欄に入ります(まだ送りません)。")
                }

                Section {
                    ForEach(BetaWelcomeContent.limitKeys, id: \.self) { key in
                        Label(L10n.tr(String.LocalizationValue(key)), systemImage: "exclamationmark.circle")
                            .font(.subheadline)
                    }
                } header: {
                    Text("まだできないこと")
                }

                Section {
                    Label("β の期間中は、作ったアプリや記録が消えることがあります。大事なデータは、ほかの場所にも残してください。",
                          systemImage: "externaldrive.badge.exclamationmark")
                        .font(.subheadline)
                } header: {
                    Text("データについて")
                }

                Section {
                    Button {
                        feedback = FeedbackContext.capture(runtime: rt, screenshot: FeedbackScreenshot.captureAppScreen())
                    } label: {
                        Label("フィードバックを送る", systemImage: "envelope")
                    }
                    .accessibilityIdentifier("betaWelcome.feedback")
                } footer: {
                    Text("困ったこと・欲しい機能を、ここからそのまま送れます。端末を振っても開けます。")
                }
            }
            .navigationTitle("β 版のご案内")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .confirmationAction) {
                    Button("はじめる") { rt.finishBetaWelcome() }
                        .accessibilityIdentifier("betaWelcome.close")
                }
            }
            .sheet(item: $feedback) { ctx in
                FeedbackComposerView(context: ctx, client: services.feedback, notifyWhenFixed: services.loopNotices.map { FeedbackFixedOptIn.live(client: $0) }) { feedback = nil }
            }
        }
        .accessibilityIdentifier("betaWelcome.sheet")
    }
}
#endif
