import Foundation
import AVFoundation
import Observation
import WhisperKit

/// WhisperKit(argmax-oss-swift)によるオンデバイス日本語音声認識。
/// 流れ: prepare(モデル DL → CoreML ロード) → startRecording(AVAudioEngine 16kHz mono) → stopAndTranscribe(停止後に一括認識)。
/// 途中結果(ストリーミング)は v0 では出さない。実機専用(シミュレータは MockTranscriber)。
@MainActor
@Observable
public final class WhisperKitRecognizer: SpeechTranscriber {
    public private(set) var status: TranscriberStatus = .notDownloaded
    public private(set) var downloadProgress: Double = 0
    public var isReady: Bool { status == .ready }

    private let variant: String
    private let repo = "argmaxinc/whisperkit-coreml"
    /// Application Support 配下に固定(Documents に 500MB を置かない)。アプリのサンドボックス内なので Bundle ID が変わると再 DL になる。
    private let downloadBase: URL
    @ObservationIgnored private var pipe: WhisperKit?
    @ObservationIgnored private var prepareTask: Task<Void, Error>?
    private var isRecording = false

    public init(model: String) {
        variant = Self.resolveVariant(model)
        let support = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first!
        downloadBase = support.appending(path: "WhisperKit", directoryHint: .isDirectory)
    }

    /// `small` → `openai_whisper-small`。既に `openai_whisper-` / `distil-` 等の接頭辞付きならそのまま。
    nonisolated public static func resolveVariant(_ model: String) -> String {
        model.contains("_") || model.hasPrefix("distil") ? model : "openai_whisper-\(model)"
    }

    /// ローカルにモデル一式が揃っているか(オフラインでも Hub へ問い合わせずに判定する)。
    private var localModelFolder: URL? {
        let hub = HubApiWrapper(downloadBase: downloadBase)
        let folder = hub.localRepoLocation(HubApiWrapper.Repo(id: repo, type: .models)).appending(path: variant)
        let required = ["MelSpectrogram.mlmodelc", "AudioEncoder.mlmodelc", "TextDecoder.mlmodelc", "config.json"]
        let ok = required.allSatisfy { FileManager.default.fileExists(atPath: folder.appending(path: $0).path) }
        return ok ? folder : nil
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
            } else {
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
            let config = WhisperKitConfig(
                modelFolder: folder.path,
                verbose: false,
                logLevel: .error,
                prewarm: true,
                load: true,
                download: false
            )
            pipe = try await WhisperKit(config)
            status = .ready
        } catch {
            status = .failed(error.localizedDescription)
            throw error
        }
    }

    public func startRecording() async throws {
        guard let pipe else { throw RecognizerError.notReady }
        guard !isRecording else { return }
        guard await AudioProcessor.requestRecordPermission() else {
            throw RecognizerError.microphoneDenied
        }
        // AudioProcessor 側で AVAudioSession(.playAndRecord) を有効化し、ハードウェアレートから 16kHz mono Float32 に変換して蓄積する。
        try pipe.audioProcessor.startRecordingLive(inputDeviceID: nil, callback: nil)
        isRecording = true
    }

    public func stopAndTranscribe() async throws -> String {
        guard let pipe else { throw RecognizerError.notReady }
        guard isRecording else { return "" }
        pipe.audioProcessor.stopRecording()
        isRecording = false
        let samples = Array(pipe.audioProcessor.audioSamples)
        pipe.audioProcessor.purgeAudioSamples(keepingLast: 0)
        // 0.5 秒未満は無音扱い(Whisper が幻聴を出しやすい)
        guard samples.count > WhisperKit.sampleRate / 2 else { return "" }

        status = .recognizing
        defer { status = .ready }
        let options = DecodingOptions(
            task: .transcribe,
            language: "ja",
            usePrefillPrompt: true,
            detectLanguage: false,
            skipSpecialTokens: true,
            withoutTimestamps: true,
            chunkingStrategy: .vad
        )
        let results = try await pipe.transcribe(audioArray: samples, decodeOptions: options)
        return results.map(\.text).joined().trimmingCharacters(in: .whitespacesAndNewlines)
    }

    public enum RecognizerError: LocalizedError {
        case notReady
        case microphoneDenied

        public var errorDescription: String? {
            switch self {
            case .notReady: return "音声モデルの準備が終わっていません"
            case .microphoneDenied: return "マイクの使用が許可されていません。設定アプリで許可してください"
            }
        }
    }
}
