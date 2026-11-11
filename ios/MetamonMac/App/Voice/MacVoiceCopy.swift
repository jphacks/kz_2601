import Foundation
import Darwin
import MetamonServices
import MetamonRenderer

/// 声で頼む(#966)の文言。iOS の `VoiceModelCopy` / `ErrorCopy.transcriberFailure` と同じ言い方で、
/// 「設定アプリ」「タップ」など iPhone の言い方だけ Mac の言い方にする。
enum MacVoiceCopy {
    static var listening: String { L10n.tr("聞いています。話し終えたら、もう一度押してください") }
    static var transcribing: String { L10n.tr("文字にしています") }
    static var provisional: String { L10n.tr("準備中のため簡易の聞き取りです") }
    static var downloadButton: String { L10n.tr("音声モデルをダウンロード") }
    static var loading: String { L10n.tr("音声モデルを読み込み中…(初回は1分ほど)") }

    static var textFirst: String {
        L10n.tr("声で話すには、声を文字にするモデル(約\(MacVoiceInput.modelMegabytes)MB)が必要です。\n下の欄に文字で入力すれば、今すぐ作れます。")
    }
    static var provisionalTextFirst: String {
        L10n.tr("今は簡易の聞き取りで話せます。音声モデル(約\(MacVoiceInput.modelMegabytes)MB)を入れると、より正確に聞き取れます。")
    }
    static func downloading(percent: Int) -> String {
        L10n.tr("音声モデルをダウンロード中 \(percent)%(初回のみ約\(MacVoiceInput.modelMegabytes)MB)\n待たなくても、下の欄に文字で入力すれば作れます")
    }

    // MARK: - ダウンロードの確認(iOS の ConsentHost と同じ形・Mac は「設定から」の一文を出さない)

    static var confirmTitle: String { L10n.tr("音声モデルをダウンロードしますか？") }

    static func confirmMessage(advice: ModelDownloadAdvice, slowHardware: Bool) -> String {
        var lines: [String] = []
        if advice.isExpensiveNetwork {
            lines.append(L10n.tr("いまは従量制の回線です。通信量が多いので、ほかの回線につないでからがおすすめです。"))
        }
        if let shortage = advice.shortageMegabytes {
            lines.append(L10n.tr("空き容量が足りません(あと約\(shortage)MB 必要です)。不要なファイルを整理してから試してください。"))
        }
        let mb = MacVoiceInput.modelMegabytes
        lines.append(L10n.pick(
            L10n.tr("話した声をこの Mac の中で文字にするためのモデルです(約\(mb)MB・初回のみ)。ダウンロード中も、終わらなくても、文字で頼めばすぐにアプリを作れます。"),
            review: L10n.tr("話した声をこの Mac の中で文字にするためのモデルです(約\(mb)MB・初回のみ)。ダウンロード中も、終わらなくても、文字で頼めばすぐに記録帳を用意できます。")))
        if slowHardware {
            lines.append(L10n.tr("この Mac では、モデルで文字にするのに時間がかかることがあります。"))
        }
        return lines.joined(separator: "\n\n")
    }

    // MARK: - 失敗

    enum PrivacyPane: String {
        case microphone = "Privacy_Microphone"
        case speechRecognition = "Privacy_SpeechRecognition"

        var url: URL { URL(string: "x-apple.systempreferences:com.apple.preference.security?\(rawValue)")! }
    }

    static var openSettings: String { L10n.tr("システム設定を開く") }

    static func message(_ f: TranscriberFailure) -> String {
        switch f {
        case .notReady: L10n.tr("音声モデルを準備中です。少し待ってから、もう一度お試しください")
        case .microphoneDenied: L10n.tr("マイクを使えません。システム設定 > プライバシーとセキュリティ > マイク で Tailor It を許可してください")
        case .audioSession, .noInputAvailable, .noAudioCaptured:
            L10n.tr("録音できませんでした。マイクがつながっているか確かめ、ほかのアプリの録音を終えてから、もう一度お試しください")
        case .tooShort: L10n.tr("うまく聞き取れませんでした。押してから話し、話し終えたらもう一度押してください")
        case .silent: L10n.tr("うまく聞き取れませんでした。マイクに近づいて、もう一度話してください")
        case .speechRecognitionDenied:
            L10n.tr("音声認識の許可がありません。システム設定 > プライバシーとセキュリティ > 音声認識 で許可するか、音声モデルの準備ができるまで文字で頼んでください")
        case .onDeviceRecognitionUnavailable: L10n.tr("この Mac では音声モデルの準備ができるまで声で頼めません。文字で頼んでください")
        }
    }

    /// 生成したアプリの話して入力(#1328)の失敗。許可が無い時はシステム設定の欄を開けるようにする
    static func dictationFailure(_ error: Error) -> DictationFailure {
        guard let f = error as? TranscriberFailure else {
            return DictationFailure(L10n.tr("録音できませんでした。もう一度お試しください"))
        }
        return DictationFailure(message(f), settingsURL: privacyPane(for: f)?.url)
    }

    static func privacyPane(for f: TranscriberFailure) -> PrivacyPane? {
        switch f {
        case .microphoneDenied: .microphone
        case .speechRecognitionDenied: .speechRecognition
        default: nil
        }
    }
}

/// 認識の速さの見込み(#363: Intel の Mac は CoreML が遅い)。Rosetta で動いていても実機の CPU を見る。
enum MacHardware {
    static let isAppleSilicon: Bool = {
        var value: Int32 = 0
        var size = MemoryLayout<Int32>.size
        return sysctlbyname("hw.optional.arm64", &value, &size, nil, 0) == 0 && value == 1
    }()
}
