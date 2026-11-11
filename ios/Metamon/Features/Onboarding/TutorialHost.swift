import SwiftUI

/// RootView に付ける: 初回のチュートリアルと、最初のアプリの直後の「こう話すと直せます」(#274)。
/// 同意の画面(ConsentHost)より後・シェアリンク/Intent より前に出す(`LaunchConsentHold`)。
struct TutorialHost: ViewModifier {
    @Environment(AppRuntime.self) private var rt

    func body(content: Content) -> some View {
        @Bindable var rt = rt
        content
            .fullScreenCover(isPresented: $rt.showTutorial, onDismiss: { rt.tutorialDismissed() }) { TutorialView() }
            .overlay(alignment: .bottom) {
                // H7: 要約バナー・確認が閉じてから(`PostBuildNotice`)
                if rt.showFirstEditHint, rt.isNoticeTurn(.editHint), !rt.dsl.isVoiceOnly, rt.morphJob == nil, !rt.showVoiceSheet {
                    FirstEditHintCard()
                        .padding(.horizontal, Theme.Spacing.md)
                        .padding(.bottom, Theme.Layout.fabBottomInset(hasTabs: rt.dsl.app.hasTabs) + Theme.Layout.fabSize + Theme.Spacing.md)
                        .transition(.move(edge: .bottom).combined(with: .opacity))
                }
            }
            .animation(.spring(duration: 0.45, bounce: 0.15), value: rt.showFirstEditHint && rt.isNoticeTurn(.editHint))
            .onChange(of: rt.showAIConsent) { _, showing in
                // 起動時以外(生成しようとして出た同意の画面)が閉じた時。起動時は下の .task が順番に出す
                guard !showing, !LaunchConsentHold.shared.pending.contains(.tutorial) else { return }
                Task { @MainActor in
                    try? await Task.sleep(for: .milliseconds(700))
                    rt.presentTutorialIfNeeded()
                }
            }
            .task {
                // 起動時: スプラッシュ → 同意の画面が閉じきってから(シートを同時に出さない)
                await rt.runLaunchTutorialStep()
            }
            .onDisappear { LaunchConsentHold.shared.end(.tutorial) } // 画面が消えた時も待たせたままにしない
    }
}
