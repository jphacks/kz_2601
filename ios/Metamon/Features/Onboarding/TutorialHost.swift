import SwiftUI

/// RootView に付ける: 初回のチュートリアルと、最初のアプリの直後の「こう話すと直せます」(#274)。
/// 同意の画面(ConsentHost)より後に出す。同意の画面が閉じた時は `AIConsentView.onDisappear` からも呼ばれる。
struct TutorialHost: ViewModifier {
    @Environment(AppRuntime.self) private var rt

    func body(content: Content) -> some View {
        @Bindable var rt = rt
        content
            .fullScreenCover(isPresented: $rt.showTutorial) { TutorialView() }
            .overlay(alignment: .bottom) {
                if rt.showFirstEditHint, !rt.dsl.isVoiceOnly, rt.morphJob == nil, !rt.showVoiceSheet {
                    FirstEditHintCard()
                        .padding(.horizontal, Theme.Spacing.md)
                        .padding(.bottom, Theme.Layout.fabBottomInset(hasTabs: rt.dsl.app.hasTabs) + Theme.Layout.fabSize + Theme.Spacing.md)
                        .transition(.move(edge: .bottom).combined(with: .opacity))
                }
            }
            .animation(.spring(duration: 0.45, bounce: 0.15), value: rt.showFirstEditHint)
            .onChange(of: rt.showAIConsent) { _, showing in
                // 同意の画面が閉じたら、閉じるアニメーションを待ってから出す(重なると表示されない)
                guard !showing else { return }
                Task { @MainActor in
                    try? await Task.sleep(for: .milliseconds(700))
                    rt.presentTutorialIfNeeded()
                }
            }
            .task {
                // 同意の説明が要らない(済み)時は、起動して少し待ってから出す(同意の画面は 0.5 秒後に出る)
                try? await Task.sleep(for: .milliseconds(800))
                rt.presentTutorialIfNeeded()
            }
    }
}
