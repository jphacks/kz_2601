import SwiftUI

/// RootView に付ける: 初回起動の同意の画面と、音声モデルのダウンロード確認(#246)。
struct ConsentHost: ViewModifier {
    @Environment(AppRuntime.self) private var rt

    func body(content: Content) -> some View {
        @Bindable var rt = rt
        content
            .sheet(isPresented: $rt.showAIConsent) { AIConsentView() }
            .alert(
                Text("音声モデルをダウンロードしますか？"),
                isPresented: Binding(get: { rt.showModelDownloadConfirm && !rt.showAIConsent },
                                     set: { if !$0 { rt.showModelDownloadConfirm = false } })
            ) {
                Button("ダウンロード") { rt.approveModelDownload() }
                Button("あとで", role: .cancel) {}
            } message: {
                Text("話した声を端末の中で文字にするためのモデルです(約\(AppRuntime.voiceModelMegabytes)MB・初回のみ)。Wi-Fi をおすすめします。ダウンロードしなくても文字入力で使えます。設定からいつでもダウンロードできます。")
            }
            .task {
                // 初回起動: まだ説明を出していなければ 1 回だけ出す(「あとで」の人には、生成しようとした時にまた出る)
                guard !rt.hasAIConsent, rt.aiConsent?.hasBeenPrompted == false else { return }
                try? await Task.sleep(for: .milliseconds(500))
                rt.showAIConsent = true
            }
    }
}
