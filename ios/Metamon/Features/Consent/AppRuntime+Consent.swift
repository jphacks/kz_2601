import Foundation
import MetamonServices

/// 第三者 AI への送信の同意(5.1.2(i))と、音声モデルのダウンロード確認(4.2.3(ii))の入口(#246)。
extension AppRuntime {
    /// 同意済みか(`aiConsent` が無い = テスト等は同意済み扱い)。
    var hasAIConsent: Bool { aiConsent?.isGranted ?? true }

    /// AI に送る直前に呼ぶ。未同意なら同意の画面を出して false(送らない)。
    func requireAIConsent() -> Bool {
        if hasAIConsent { return true }
        print("[Metamon] AI consent not granted: not sending")
        guard showVoiceSheet else { showAIConsent = true; return false }
        // 音声シートを閉じきってから出す(シートの同時表示は SwiftUI が無視する)
        showVoiceSheet = false
        Task { @MainActor [weak self] in
            try? await Task.sleep(for: .milliseconds(600))
            self?.showAIConsent = true
        }
        return false
    }

    func grantAIConsent() {
        aiConsent?.grant()
        showAIConsent = false
    }

    /// 取り消し(設定から)。以後の生成・対話は送らない。
    func revokeAIConsent() {
        aiConsent?.revoke()
    }

    /// 起動時: モデルが端末にあれば温める。無ければサイズを示して確認してから(承認済みなら聞かない)。
    func prepareTranscriberIfAllowed() {
        switch ModelDownloadConsent.decide(needsDownload: services.transcriber.needsDownload) {
        case .prepare:
            Task { try? await services.transcriber.prepare() }
        case .ask:
            print("[Metamon] voice model not on device: asking before download")
            showModelDownloadConfirm = true
        }
    }

    /// 確認で「ダウンロード」を選んだ時。
    func approveModelDownload() {
        ModelDownloadConsent.approve()
        showModelDownloadConfirm = false
        Task { try? await services.transcriber.prepare() }
    }

    /// 確認の文言に出すおおよそのサイズ(MB)。
    static var voiceModelMegabytes: Int { ModelDownloadConsent.approximateMegabytes(model: BuildConfig.whisperModel) }
}
