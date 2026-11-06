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

    /// 同意の画面が閉じた時(`ConsentHost`)。同意していれば、同意の前に話した 1 件を送り直す(M2: 話し直さなくてよい)。
    /// 「あとで」で閉じたら捨てる(後で設定から同意した時に、古い発話を勝手に送らない)。
    func resumeUtteranceAfterConsent() async {
        guard let pending = utteranceAwaitingConsent else { return }
        utteranceAwaitingConsent = nil
        guard hasAIConsent else { return }
        print("[Metamon] AI consent granted → sending the utterance held before consent")
        if !dsl.isVoiceOnly, routesToChat(pending.text) { showVoiceSheet = true } // 作ったアプリの修正の対話は音声シートの中
        await submitUtterance(pending.text, source: pending.source)
    }

    /// 取り消し(設定から)。以後の生成・対話は送らない。
    func revokeAIConsent() {
        aiConsent?.revoke()
    }

    /// 起動時: モデルが端末にあれば温める(承認済みならダウンロードも始める)。
    /// 無くて未承認なら**起動では聞かない**(M14・H1: 起動のたびのアラートをやめる)。初期画面のマイクの下の案内
    /// (「音声モデルをダウンロード」)と、マイクを押した時(`askModelDownloadBeforeRecording`)に確認する。
    func prepareTranscriberIfAllowed(defaults: UserDefaults = .standard) {
        switch ModelDownloadConsent.decide(needsDownload: services.transcriber.needsDownload,
                                           isExpensiveNetwork: ExpensiveNetworkProbe.shared.isExpensive, // #275
                                           defaults: defaults) {
        case .prepare:
            Task { try? await services.transcriber.prepare() }
        case .ask:
            print("[Metamon] voice model not on device: waiting for the user (mic / download button)")
        }
    }

    /// マイクを押した時: モデルが無く、まだダウンロードを承認していなければ確認を出して true(録音しない・M14)。
    /// 音声シートの中では root のアラートが出せないので聞かない(従来どおり録音を試みる)。
    func askModelDownloadBeforeRecording(defaults: UserDefaults = .standard) -> Bool {
        guard !showVoiceSheet, services.transcriber.status == .notDownloaded,
              ModelDownloadConsent.decide(needsDownload: services.transcriber.needsDownload,
                                          isExpensiveNetwork: ExpensiveNetworkProbe.shared.isExpensive,
                                          defaults: defaults) == .ask else { return false }
        print("[Metamon] mic tapped without the voice model: asking before download")
        showModelDownloadConfirm = true
        return true
    }

    /// 確認で「ダウンロード」を選んだ時。
    func approveModelDownload() {
        ModelDownloadConsent.approve()
        showModelDownloadConfirm = false
        Task { try? await services.transcriber.prepare() }
    }

    /// 確認に添える注意(モバイル回線・空き容量・#275)。確認を出す時に読む。
    var modelDownloadAdvice: ModelDownloadAdvice {
        ModelDownloadAdvice.evaluate(megabytes: Self.voiceModelMegabytes,
                                     isExpensiveNetwork: ExpensiveNetworkProbe.shared.isExpensive,
                                     availableBytes: ModelDownloadAdvice.availableBytesOnDevice())
    }

    /// 確認の文言に出すおおよそのサイズ(MB)。
    static var voiceModelMegabytes: Int { ModelDownloadConsent.approximateMegabytes(model: BuildConfig.whisperModel) }
}
