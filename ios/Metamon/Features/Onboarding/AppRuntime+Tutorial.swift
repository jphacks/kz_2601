import Foundation
import MetamonServices

/// 初回のチュートリアルと「こう言うと直せます」(#274)。
extension AppRuntime {
    /// チュートリアルを見たか(`tutorial` が無い = テスト等は見た扱い)。
    var hasSeenTutorial: Bool { tutorial?.hasSeenTutorial ?? true }

    /// 起動時・同意の画面が閉じた時に呼ぶ。同意の説明を出し終えていて、まだ見ていなければ出す。
    func presentTutorialIfNeeded() {
        guard let tutorial, !showTutorial, !showAIConsent,
              tutorial.shouldAutoPresent(consentPrompted: hasAIConsent || aiConsent?.hasBeenPrompted ?? true) else { return }
        print("[Metamon] tutorial: presenting first-run tutorial")
        showTutorial = true
    }

    /// 設定の「使い方を見る」。シートを閉じきってから出す(シートの同時表示は SwiftUI が無視する)。
    func replayTutorial() {
        showVoiceSheet = false
        Task { @MainActor [weak self] in
            try? await Task.sleep(for: .milliseconds(600))
            self?.showTutorial = true
        }
    }

    /// スキップ・「はじめる」のどちらでも見た扱い(次から自動では出さない)。
    func finishTutorial() {
        tutorial?.markTutorialSeen()
        showTutorial = false
    }

    /// 初期画面から最初のアプリができた直後に 1 回だけ、要約カードが消えた頃に直し方の例を出す。
    func scheduleFirstEditHint(createdFromInitial: Bool) {
        guard let tutorial, tutorial.shouldShowEditHint(createdFromInitial: createdFromInitial) else { return }
        tutorial.markEditHintShown()
        Task { @MainActor [weak self] in
            try? await Task.sleep(for: .seconds(Self.editHintDelaySeconds))
            guard let self, !dsl.isVoiceOnly, morphJob == nil else { return }
            showFirstEditHint = true
        }
    }

    /// 要約カード(3〜6 秒)と重ならない待ち時間。
    static let editHintDelaySeconds: Double = 6.5

    /// 直し方の例のボタン: 発話シートを開く。
    func openVoiceFromEditHint() {
        showFirstEditHint = false
        showVoiceSheet = true
    }

    /// 音声モデルのダウンロード確認は、同意 → チュートリアルが済んでから出す(初回の順序・#274)。
    var canShowModelDownloadConfirm: Bool {
        showModelDownloadConfirm && !showAIConsent && !showTutorial && hasSeenTutorial
    }
}
