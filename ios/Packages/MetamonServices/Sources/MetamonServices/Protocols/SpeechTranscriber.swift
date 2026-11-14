import Foundation

/// 音声認識モデルの状態(UI 表示用)。
public enum TranscriberStatus: Equatable, Sendable {
    /// モデル未ダウンロード(prepare 前)
    case notDownloaded
    /// ダウンロード中(downloadProgress 0...1)
    case downloading
    /// ダウンロード済み・CoreML ロード/特化中
    case loading
    /// 録音・文字起こし可能
    case ready
    /// 文字起こし実行中
    case recognizing
    /// 準備失敗(prepare を再試行できる)
    case failed(String)
}

/// 音声入力。実機は `WhisperKitRecognizer`、シミュレータ/テストは `MockTranscriber`。
@MainActor
public protocol SpeechTranscriber: AnyObject {
    var isReady: Bool { get }
    var status: TranscriberStatus { get }
    /// モデル DL 進捗 0...1(downloading 以外は意味を持たない)
    var downloadProgress: Double { get }
    /// モデル DL / ロード。初回は時間がかかるのでアプリ起動直後に非同期で呼ぶ。
    func prepare() async throws
    func startRecording() async throws
    /// 録音を止めて文字起こし。短すぎ/無音/マイク未入力は `TranscriberFailure`(理由付き)を throw し、
    /// 認識はできたが文字が出なかった時だけ "" を返す(理由は `lastDiagnostics` に残る)。
    func stopAndTranscribe() async throws -> String
    /// 録音を捨てて止める(文字起こしはしない)。録音中に画面が閉じられた時など。録音していなければ何もしない。
    func cancelRecording()
    /// 直近の録音/認識の診断(録音秒数・サンプル数・音量・AVAudioSession・権限・モデル状態)。未実装なら nil。
    var lastDiagnostics: TranscriberDiagnostics? { get }
    /// 端末にモデルが無く、prepare するとダウンロードが始まるか(#246: その前にサイズを示して確認する)。
    var needsDownload: Bool { get }
    /// 本命のモデルの準備中で、仮の聞き取り(端末内の Apple の音声認識・U5)を使っているか。画面に「簡易の聞き取り」と出す。
    var isProvisional: Bool { get }
}

public extension SpeechTranscriber {
    var status: TranscriberStatus { isReady ? .ready : .loading }
    var downloadProgress: Double { 0 }
    var lastDiagnostics: TranscriberDiagnostics? { nil }
    var needsDownload: Bool { false }
    var isProvisional: Bool { false }
    func cancelRecording() {}
}

/// 「音声を聞き取れませんでした」の理由を画面とログに出すための診断情報。
public struct TranscriberDiagnostics: Equatable, Sendable {
    public var recordedSeconds: Double = 0
    public var sampleCount: Int = 0
    /// 0...1 のピーク振幅(全サンプルの最大絶対値)。0 ならマイクから音が一切届いていない。
    public var peakAmplitude: Float = 0
    public var rmsAmplitude: Float = 0
    public var sessionCategory: String = ""
    public var sessionMode: String = ""
    public var inputRoute: String = ""
    public var inputSampleRate: Double = 0
    public var inputAvailable = false
    public var micPermission: String = ""
    public var modelStatus: String = ""
    public var modelFolderExists = false
    public var transcriptLength = 0
    public var failure: String?

    public init() {}

    /// 音量(dBFS)。無音なら -inf。
    public var peakDecibels: Float { 20 * log10(max(peakAmplitude, 1e-6)) }

    /// toast に添える 1 行要約。例: `録音2.3秒・音量-28dB・入力:iPhone Microphone`
    public var summary: String {
        var parts = [L("録音\(recordedSeconds, specifier: "%.1f")秒")]
        parts.append(peakAmplitude > 0 ? L("音量\(peakDecibels, specifier: "%.0f")dB") : L("無音"))
        if !inputRoute.isEmpty { parts.append(L("入力:\(inputRoute)")) }
        return parts.joined(separator: L("・"))
    }
}

/// 録音/認識が失敗した理由。`errorDescription` はそのまま toast に出せる日本語。
public enum TranscriberFailure: LocalizedError, Equatable, Sendable {
    case notReady(String)
    case microphoneDenied
    case audioSession(String)
    case noInputAvailable
    /// 録音は始まったがマイクから 1 サンプルも届かなかった(オーディオセッションを他が握っている等)
    case noAudioCaptured(seconds: Double)
    case tooShort(seconds: Double)
    /// サンプルは届いたが全て 0 付近(ミュート/マイク故障/入力ルート不正)
    case silent(seconds: Double, peakDecibels: Float)
    /// 仮の聞き取り(Apple の端末内の音声認識)の許可が無い(U5)。文字の入力を案内する
    case speechRecognitionDenied
    /// この端末・言語では端末内の音声認識が使えない(サーバーへ送る認識は使わない・U5)
    case onDeviceRecognitionUnavailable

    public var errorDescription: String? {
        switch self {
        case .notReady(let status): return L("音声モデルを準備中です(\(status))。少し待ってからお試しください")
        case .microphoneDenied: return L("マイクの権限がありません。設定アプリ > Tailor It でマイクを許可してください")
        case .audioSession(let reason): return L("録音を開始できませんでした(オーディオセッション: \(reason))")
        case .noInputAvailable: return L("使えるマイクが見つかりません(他のアプリが使用中か、入力ルートがありません)")
        case .noAudioCaptured(let s): return L("マイクから音声が届きませんでした(録音\(s, specifier: "%.1f")秒・0サンプル)。他のアプリの通話/録音を終了して再試行してください")
        case .tooShort(let s): return L("録音が短すぎました(\(s, specifier: "%.1f")秒)。タップしてから話し、話し終えてからもう一度タップしてください")
        case .silent(let s, let db): return L("音声が小さすぎました(録音\(s, specifier: "%.1f")秒・音量\(db, specifier: "%.0f")dB)。マイクに向かって話してください")
        case .speechRecognitionDenied: return L("音声認識の許可がありません。音声モデルの準備ができるまでは、文字で入力してください")
        case .onDeviceRecognitionUnavailable: return L("この端末では音声モデルの準備ができるまで声で入力できません。文字で入力してください")
        }
    }
}
