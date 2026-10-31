import SwiftUI
import MetamonServices

/// RootView に付ける: 初回起動の同意の画面と、音声モデルのダウンロード確認(#246)。
struct ConsentHost: ViewModifier {
    @Environment(AppRuntime.self) private var rt

    func body(content: Content) -> some View {
        @Bindable var rt = rt
        content
            .sheet(isPresented: $rt.showAIConsent) { AIConsentView() }
            .alert(
                Text("音声モデルをダウンロードしますか？"),
                isPresented: Binding(get: { rt.canShowModelDownloadConfirm }, // 同意 → チュートリアル(#274)の後
                                     set: { if !$0 { rt.showModelDownloadConfirm = false } })
            ) {
                Button("ダウンロード") { rt.approveModelDownload() }
                Button("あとで", role: .cancel) {}
            } message: {
                Text(Self.downloadMessage(advice: rt.modelDownloadAdvice))
            }
            .task {
                // 初回起動: まだ説明を出していなければ 1 回だけ出す(「あとで」の人には、生成しようとした時にまた出る)
                guard !rt.hasAIConsent, rt.aiConsent?.hasBeenPrompted == false else { return }
                try? await Task.sleep(for: .milliseconds(500))
                rt.showAIConsent = true
            }
    }

    /// ダウンロード確認の本文。モバイル回線・空き容量不足の時は先頭に注意を足す(#275)。
    static func downloadMessage(advice: ModelDownloadAdvice) -> String {
        var lines: [String] = []
        if advice.isExpensiveNetwork {
            lines.append(L10n.tr("いまはモバイル回線です。通信量が多いので、Wi-Fi につないでからがおすすめです。"))
        }
        if let shortage = advice.shortageMegabytes {
            lines.append(L10n.tr("空き容量が足りません(あと約\(shortage)MB 必要です)。写真やアプリを整理してから試してください。"))
        }
        lines.append(L10n.pick(L10n.tr("話した声を端末の中で文字にするためのモデルです(約\(AppRuntime.voiceModelMegabytes)MB・初回のみ)。ダウンロード中も、終わらなくても、文字入力ならすぐにアプリを作れます。設定からいつでもダウンロードできます。"), review: L10n.tr("話した声を端末の中で文字にするためのモデルです(約\(AppRuntime.voiceModelMegabytes)MB・初回のみ)。ダウンロード中も、終わらなくても、文字入力ならすぐに記録帳を用意できます。設定からいつでもダウンロードできます。")))
        return lines.joined(separator: "\n\n")
    }
}
