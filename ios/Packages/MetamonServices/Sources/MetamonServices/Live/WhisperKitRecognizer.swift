import Foundation
import AVFoundation
import Accelerate
import Observation
import OSLog
import WhisperKit

/// WhisperKit(argmax-oss-swift)によるオンデバイス日本語音声認識。
/// 流れ: prepare(モデル DL → CoreML ロード) → startRecording(AVAudioEngine 16kHz mono) → stopAndTranscribe(停止後に一括認識)。
/// 途中結果(ストリーミング)は v0 では出さない。実機専用(シミュレータは MockTranscriber)。
///
/// 「聞き取れませんでした」の切り分けのため、録音前後の AVAudioSession / 権限 / サンプル数 / 音量を `lastDiagnostics` に残し、
/// `[Voice]` プレフィックスで stdout(`devicectl device process launch --console`)と os_log(category: voice)に出す。
@MainActor
@Observable
public final class WhisperKitRecognizer: SpeechTranscriber {
    public private(set) var status: TranscriberStatus = .notDownloaded
    public private(set) var downloadProgress: Double = 0
    public private(set) var lastDiagnostics: TranscriberDiagnostics?
    public var isReady: Bool { status == .ready }

    /// これ未満の録音は無音扱い(Whisper が幻聴を出しやすい)
    nonisolated public static let minimumSeconds: Double = 0.5
    /// ピークがこれ未満(約 -60dBFS)なら「音が入っていない」扱い
    nonisolated public static let silencePeak: Float = 0.001

    private let variant: String
    private let repo = "argmaxinc/whisperkit-coreml"
    /// Application Support 配下に固定(Documents に 500MB を置かない)。アプリのサンドボックス内なので Bundle ID が変わると再 DL になる。
    private let downloadBase: URL
    @ObservationIgnored private var pipe: WhisperKit?
    @ObservationIgnored private var prepareTask: Task<Void, Error>?
    @ObservationIgnored private var recordingStartedAt: Date?
    @ObservationIgnored private var watchdog: Task<Void, Never>?
    @ObservationIgnored private let logger = Logger(subsystem: "jp.kz2601.metamon", category: "voice")
    private var isRecording = false
    /// 認識言語("ja" | "en")。録音ごとに読むので、アプリの言語設定を切り替えると次の発話から反映される(#45)。
    @ObservationIgnored public var language: () -> String = { "ja" }

    public init(model: String) {
        variant = Self.resolveVariant(model)
        let support = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first!
        downloadBase = support.appending(path: "WhisperKit", directoryHint: .isDirectory)
    }

    /// `small` → `openai_whisper-small`。既に `openai_whisper-` / `distil-` 等の接頭辞付きならそのまま。
    nonisolated public static func resolveVariant(_ model: String) -> String {
        model.contains("_") || model.hasPrefix("distil") ? model : "openai_whisper-\(model)"
    }

    /// 録音結果を認識に回す前の判定。0 サンプル / 短すぎ / 無音は理由付きで失敗にする(単体テスト対象)。
    nonisolated public static func precheck(sampleCount: Int, seconds: Double, peak: Float) -> TranscriberFailure? {
        if sampleCount == 0 { return .noAudioCaptured(seconds: seconds) }
        if Double(sampleCount) < Double(WhisperKit.sampleRate) * minimumSeconds { return .tooShort(seconds: seconds) }
        if peak < silencePeak { return .silent(seconds: seconds, peakDecibels: 20 * log10(max(peak, 1e-6))) }
        return nil
    }

    /// ローカルにモデル一式が揃っているか(オフラインでも Hub へ問い合わせずに判定する)。
    private var localModelFolder: URL? {
        let hub = HubApiWrapper(downloadBase: downloadBase)
        let folder = hub.localRepoLocation(HubApiWrapper.Repo(id: repo, type: .models)).appending(path: variant)
        let required = ["MelSpectrogram.mlmodelc", "AudioEncoder.mlmodelc", "TextDecoder.mlmodelc", "config.json"]
        let ok = required.allSatisfy { FileManager.default.fileExists(atPath: folder.appending(path: $0).path) }
        return ok ? folder : nil
    }

    /// モデル未取得で、prepare がダウンロードを始めるか(#246)。
    public var needsDownload: Bool { pipe == nil && localModelFolder == nil }

    private func log(_ message: String) {
        print("[Voice] \(message)")
        logger.notice("\(message, privacy: .public)")
    }

    // MARK: - SpeechTranscriber

    /// 多重呼び出しは同じ Task を待つ。失敗したら次回呼び出しで再試行。
    public func prepare() async throws {
        if pipe != nil { return }
        if let running = prepareTask { return try await running.value }
        let task = Task { try await self.prepareImpl() }
        prepareTask = task
        defer { prepareTask = nil }
        try await task.value
    }

    private func prepareImpl() async throws {
        do {
            let folder: URL
            if let local = localModelFolder {
                folder = local
                log("model found locally: \(local.path)")
            } else {
                log("model not found locally, downloading \(variant) to \(downloadBase.path)")
                status = .downloading
                downloadProgress = 0
                folder = try await WhisperKit.download(
                    variant: variant,
                    downloadBase: downloadBase,
                    from: repo,
                    progressCallback: { [weak self] progress in
                        let value = progress.fractionCompleted
                        Task { @MainActor [weak self] in self?.downloadProgress = value }
                    }
                )
                downloadProgress = 1
            }
            status = .loading
            // modelFolder 指定 + download: false → Hub へは一切アクセスしない。prewarm で ANE 向けに特化(初回のみ数十秒)。
            // verbose: true + .error で WhisperKit 内部のエラー(リサンプル失敗等)だけ標準出力に出す(false だと一切出ない)。
            let config = WhisperKitConfig(
                modelFolder: folder.path,
                verbose: true,
                logLevel: .error,
                prewarm: true,
                load: true,
                download: false
            )
            pipe = try await WhisperKit(config)
            status = .ready
            log("model ready: \(variant)")
        } catch {
            status = .failed(error.localizedDescription)
            log("prepare failed: \(error)")
            throw error
        }
    }

    public func startRecording() async throws {
        var diag = TranscriberDiagnostics()
        diag.modelStatus = "\(status)"
        diag.modelFolderExists = localModelFolder != nil
        defer { lastDiagnostics = diag }

        guard let pipe else {
            diag.failure = "notReady"
            throw TranscriberFailure.notReady("\(status)")
        }
        guard !isRecording else { return }

        // 1. マイク権限(未決定ならここでダイアログ)
        diag.micPermission = "\(AVAudioApplication.shared.recordPermission.rawValue)"
        guard await AudioProcessor.requestRecordPermission() else {
            diag.failure = "microphoneDenied"
            log("start failed: microphone denied")
            throw TranscriberFailure.microphoneDenied
        }
        diag.micPermission = "granted"

        // 2. AVAudioSession を自前で明示的に有効化(WhisperKit 側は try? で失敗を握りつぶすため)。
        //    カメラ(UIImagePickerController / AVCaptureSession)の後でも必ず record 系カテゴリに戻す。
        let session = AVAudioSession.sharedInstance()
        let before = "\(session.category.rawValue)/\(session.mode.rawValue)"
        do {
            try session.setCategory(.playAndRecord, mode: .default, options: [.defaultToSpeaker, .allowBluetooth])
            try session.setActive(true, options: [])
        } catch {
            diag.failure = "audioSession: \(error.localizedDescription)"
            log("start failed: audio session \(error)")
            throw TranscriberFailure.audioSession(error.localizedDescription)
        }
        diag.sessionCategory = session.category.rawValue
        diag.sessionMode = session.mode.rawValue
        diag.inputAvailable = session.isInputAvailable
        diag.inputSampleRate = session.sampleRate
        diag.inputRoute = session.currentRoute.inputs.map(\.portName).joined(separator: ",")
        log("session before=\(before) after=\(diag.sessionCategory)/\(diag.sessionMode) inputAvailable=\(diag.inputAvailable) rate=\(diag.inputSampleRate) input=[\(diag.inputRoute)] channels=\(session.inputNumberOfChannels)")
        guard session.isInputAvailable else {
            diag.failure = "noInputAvailable"
            throw TranscriberFailure.noInputAvailable
        }

        // 3. 録音開始(ハードウェアレート → 16kHz mono Float32 に変換して蓄積)
        do {
            try pipe.audioProcessor.startRecordingLive(inputDeviceID: nil, callback: nil)
        } catch {
            diag.failure = "startRecordingLive: \(error.localizedDescription)"
            log("start failed: startRecordingLive \(error)")
            throw TranscriberFailure.audioSession(error.localizedDescription)
        }
        isRecording = true
        recordingStartedAt = Date()
        let engineRunning = (pipe.audioProcessor as? AudioProcessor)?.audioEngine?.isRunning ?? false
        log("recording started engineRunning=\(engineRunning)")

        // 0.7 秒後に 1 バッファも届いていなければログ(停止時の 0 サンプル判定の裏取り用)
        watchdog?.cancel()
        watchdog = Task { [weak self] in
            try? await Task.sleep(for: .milliseconds(700))
            guard let self, !Task.isCancelled, self.isRecording else { return }
            let n = self.pipe?.audioProcessor.audioSamples.count ?? 0
            self.log(n == 0 ? "WARNING no audio buffers after 0.7s (tap not firing?)" : "first buffers ok: \(n) samples after 0.7s")
        }
    }

    public func stopAndTranscribe() async throws -> String {
        guard let pipe else { throw TranscriberFailure.notReady("\(status)") }
        guard isRecording else { return "" }
        watchdog?.cancel()
        let engineRunning = (pipe.audioProcessor as? AudioProcessor)?.audioEngine?.isRunning ?? false
        pipe.audioProcessor.stopRecording()
        isRecording = false
        let samples = Array(pipe.audioProcessor.audioSamples)
        pipe.audioProcessor.purgeAudioSamples(keepingLast: 0)

        var diag = lastDiagnostics ?? TranscriberDiagnostics()
        diag.recordedSeconds = recordingStartedAt.map { Date().timeIntervalSince($0) } ?? 0
        diag.sampleCount = samples.count
        (diag.peakAmplitude, diag.rmsAmplitude) = Self.levels(of: samples)
        diag.failure = nil
        recordingStartedAt = nil
        log(String(format: "stopped engineWasRunning=%@ samples=%d duration=%.2fs (audio=%.2fs) peak=%.4f (%.0fdB) rms=%.5f",
                   engineRunning ? "true" : "false", samples.count, diag.recordedSeconds,
                   Double(samples.count) / Double(WhisperKit.sampleRate), diag.peakAmplitude, diag.peakDecibels, diag.rmsAmplitude))

        if let failure = Self.precheck(sampleCount: samples.count, seconds: diag.recordedSeconds, peak: diag.peakAmplitude) {
            diag.failure = "\(failure)"
            lastDiagnostics = diag
            log("precheck failed: \(failure)")
            throw failure
        }

        status = .recognizing
        defer { status = .ready }
        let options = DecodingOptions(
            task: .transcribe,
            language: language(),
            usePrefillPrompt: true,
            detectLanguage: false,
            skipSpecialTokens: true,
            withoutTimestamps: true,
            chunkingStrategy: .vad
        )
        do {
            let results = try await pipe.transcribe(audioArray: samples, decodeOptions: options)
            let text = results.map(\.text).joined().trimmingCharacters(in: .whitespacesAndNewlines)
            diag.transcriptLength = text.count
            if text.isEmpty { diag.failure = "emptyTranscript" }
            lastDiagnostics = diag
            log("transcribe result segments=\(results.flatMap(\.segments).count) text=\"\(text)\"")
            return text
        } catch {
            diag.failure = "transcribe: \(error.localizedDescription)"
            lastDiagnostics = diag
            log("transcribe failed: \(error)")
            throw error
        }
    }

    /// (ピーク振幅, RMS)。空なら (0, 0)。
    nonisolated static func levels(of samples: [Float]) -> (Float, Float) {
        guard !samples.isEmpty else { return (0, 0) }
        var peak: Float = 0
        var rms: Float = 0
        vDSP_maxmgv(samples, 1, &peak, vDSP_Length(samples.count))
        vDSP_rmsqv(samples, 1, &rms, vDSP_Length(samples.count))
        return (peak, rms)
    }
}
