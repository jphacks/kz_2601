import Foundation
import AVFoundation
import Observation
import OSLog
import Speech

/// 仮の聞き取りの能力(U5)。本命(WhisperKit)の準備中だけ `FallbackTranscriber` が使う。
@MainActor
public protocol ProvisionalTranscriber: SpeechTranscriber {
    /// 今この端末・言語・許可で使えるか。false なら使わない(文字の入力を先に出す)。
    var isAvailable: Bool { get }
}

/// 音声認識の許可(`SFSpeechRecognizerAuthorizationStatus` をテストで差し替えられる形に)。
public enum SpeechAuthorization: Equatable, Sendable {
    case notDetermined, authorized, denied, restricted
}

/// 端末内の認識が使えるか・許可の取り方。テストでは偽物を渡す。
public struct OnDeviceSpeechSupport {
    public var authorization: @MainActor () -> SpeechAuthorization
    public var requestAuthorization: @MainActor () async -> SpeechAuthorization
    /// その言語("ja" | "en")で `requiresOnDeviceRecognition` が使えるか
    public var supportsOnDevice: @MainActor (String) -> Bool

    public init(authorization: @escaping @MainActor () -> SpeechAuthorization,
                requestAuthorization: @escaping @MainActor () async -> SpeechAuthorization,
                supportsOnDevice: @escaping @MainActor (String) -> Bool) {
        self.authorization = authorization
        self.requestAuthorization = requestAuthorization
        self.supportsOnDevice = supportsOnDevice
    }

    public static var system: OnDeviceSpeechSupport { OnDeviceSpeechSupport(
        authorization: { SpeechAuthorization(SFSpeechRecognizer.authorizationStatus()) },
        requestAuthorization: {
            await withCheckedContinuation { cont in
                SFSpeechRecognizer.requestAuthorization { cont.resume(returning: SpeechAuthorization($0)) }
            }
        },
        supportsOnDevice: { AppleOnDeviceTranscriber.recognizer(for: $0)?.supportsOnDeviceRecognition ?? false }
    ) }
}

extension SpeechAuthorization {
    init(_ status: SFSpeechRecognizerAuthorizationStatus) {
        switch status {
        case .authorized: self = .authorized
        case .denied: self = .denied
        case .restricted: self = .restricted
        default: self = .notDetermined
        }
    }
}

/// Apple の端末内の音声認識(`SFSpeechRecognizer`・`requiresOnDeviceRecognition = true`)。U5: WhisperKit の準備中だけの仮の聞き取り。
/// 端末内で使えない端末・言語では使わない(サーバーへ送る Apple の認識には落とさない)。声はどこにも送らず、録音も残さない。
@MainActor
@Observable
public final class AppleOnDeviceTranscriber: ProvisionalTranscriber {
    public private(set) var authorization: SpeechAuthorization
    public private(set) var lastDiagnostics: TranscriberDiagnostics?
    /// 録音・聞き取りの途中か
    public private(set) var isRecording = false

    /// これ未満の録音は短すぎ扱い(WhisperKit と同じ)
    nonisolated public static let minimumSeconds: Double = 0.5
    /// ピークがこれ未満なら無音扱い(WhisperKitRecognizer.silencePeak と同じ)
    nonisolated public static let silencePeak: Float = 0.001
    /// 止めてから結果を待つ上限。超えたら途中の結果を使う
    nonisolated static let finalResultTimeout: Duration = .seconds(8)

    @ObservationIgnored private let support: OnDeviceSpeechSupport
    @ObservationIgnored public var language: () -> String
    @ObservationIgnored private var engine: AVAudioEngine?
    @ObservationIgnored private var request: SFSpeechAudioBufferRecognitionRequest?
    @ObservationIgnored private var task: SFSpeechRecognitionTask?
    @ObservationIgnored private var box: RecognitionBox?
    @ObservationIgnored private var meter: AudioMeter?
    @ObservationIgnored private var recordingStartedAt: Date?
    @ObservationIgnored private let logger = Logger(subsystem: "jp.kz2601.metamon", category: "voice")

    public init(language: @escaping () -> String = { "ja" }, support: OnDeviceSpeechSupport = .system) {
        self.language = language
        self.support = support
        authorization = support.authorization()
    }

    nonisolated static func localeID(for language: String) -> String { language == "en" ? "en-US" : "ja-JP" }

    nonisolated static func recognizer(for language: String) -> SFSpeechRecognizer? {
        SFSpeechRecognizer(locale: Locale(identifier: localeID(for: language)))
    }

    /// 端末内で使え、許可が拒否されていない(未決定なら最初の録音で聞く)
    public var isAvailable: Bool {
        guard authorization != .denied, authorization != .restricted else { return false }
        return supportsOnDevice(language())
    }

    /// 画面の描画ごとに SFSpeechRecognizer を作らないよう、使えると分かった言語は覚えておく
    @ObservationIgnored private var supportedLanguages: Set<String> = []
    private func supportsOnDevice(_ lang: String) -> Bool {
        if supportedLanguages.contains(lang) { return true }
        let ok = support.supportsOnDevice(lang)
        if ok { supportedLanguages.insert(lang) }
        return ok
    }

    public var isReady: Bool { isAvailable }
    public var status: TranscriberStatus { isAvailable ? (isRecording ? .recognizing : .ready) : .failed("onDeviceUnavailable") }
    public var isProvisional: Bool { true }

    private func log(_ message: String) {
        print("[Voice] [apple] \(message)")
        logger.notice("apple: \(message, privacy: .public)")
    }

    public func prepare() async throws {}

    /// 録音前の確認(許可・端末内で使えるか)。音声の部分には触らないので単体テストできる。
    func checkBeforeRecording() async throws -> String {
        let lang = language()
        guard support.supportsOnDevice(lang) else { throw TranscriberFailure.onDeviceRecognitionUnavailable }
        authorization = support.authorization()
        if authorization == .notDetermined {
            authorization = await support.requestAuthorization()
        }
        guard authorization == .authorized else { throw TranscriberFailure.speechRecognitionDenied }
        return lang
    }

    public func startRecording() async throws {
        guard !isRecording else { return }
        var diag = TranscriberDiagnostics()
        diag.modelStatus = "appleOnDevice"
        defer { lastDiagnostics = diag }

        let lang: String
        do {
            lang = try await checkBeforeRecording()
        } catch {
            diag.failure = "\(error)"
            log("start failed: \(error)")
            throw error
        }
        guard let recognizer = Self.recognizer(for: lang), recognizer.supportsOnDeviceRecognition else {
            diag.failure = "onDeviceRecognitionUnavailable"
            throw TranscriberFailure.onDeviceRecognitionUnavailable
        }

        guard await AVAudioApplication.requestRecordPermission() else {
            diag.failure = "microphoneDenied"
            throw TranscriberFailure.microphoneDenied
        }
        diag.micPermission = "granted"

        #if os(iOS)
        let session = AVAudioSession.sharedInstance()
        do {
            try session.setCategory(.playAndRecord, mode: .default, options: [.defaultToSpeaker, .allowBluetoothHFP])
            try session.setActive(true, options: [])
        } catch {
            diag.failure = "audioSession: \(error.localizedDescription)"
            throw TranscriberFailure.audioSession(error.localizedDescription)
        }
        diag.sessionCategory = session.category.rawValue
        diag.sessionMode = session.mode.rawValue
        diag.inputAvailable = session.isInputAvailable
        diag.inputSampleRate = session.sampleRate
        diag.inputRoute = session.currentRoute.inputs.map(\.portName).joined(separator: ",")
        guard session.isInputAvailable else {
            diag.failure = "noInputAvailable"
            throw TranscriberFailure.noInputAvailable
        }
        #else
        MacAudioInput.fill(&diag)
        guard diag.inputAvailable else {
            diag.failure = "noInputAvailable"
            throw TranscriberFailure.noInputAvailable
        }
        #endif

        let request = SFSpeechAudioBufferRecognitionRequest()
        request.requiresOnDeviceRecognition = true // 声を端末の外に出さない(U5 の条件)
        request.shouldReportPartialResults = true
        request.addsPunctuation = true
        request.taskHint = .dictation

        let engine = AVAudioEngine()
        let input = engine.inputNode
        let format = input.outputFormat(forBus: 0)
        let meter = AudioMeter()
        input.installTap(onBus: 0, bufferSize: 1024, format: format, block: Self.makeTap(request: request, meter: meter))
        engine.prepare()
        do {
            try engine.start()
        } catch {
            input.removeTap(onBus: 0)
            diag.failure = "engine: \(error.localizedDescription)"
            throw TranscriberFailure.audioSession(error.localizedDescription)
        }

        let box = RecognitionBox()
        task = recognizer.recognitionTask(with: request, resultHandler: Self.makeResultHandler(box: box))
        self.engine = engine
        self.request = request
        self.meter = meter
        self.box = box
        recordingStartedAt = Date()
        isRecording = true
        log("recording started locale=\(Self.localeID(for: lang)) rate=\(format.sampleRate)")
    }

    public func cancelRecording() {
        guard isRecording else { return }
        task?.cancel()
        tearDown()
        log("recording cancelled")
    }

    public func stopAndTranscribe() async throws -> String {
        guard isRecording, let request, let box, let meter else { return "" }
        engine?.stop()
        engine?.inputNode.removeTap(onBus: 0)
        request.endAudio()

        var diag = lastDiagnostics ?? TranscriberDiagnostics()
        let seconds = recordingStartedAt.map { Date().timeIntervalSince($0) } ?? 0
        let level = meter.snapshot()
        diag.recordedSeconds = seconds
        diag.sampleCount = level.frames
        diag.peakAmplitude = level.peak
        diag.failure = nil

        if let failure = Self.precheck(frames: level.frames, seconds: seconds, peak: level.peak) {
            task?.cancel()
            tearDown()
            diag.failure = "\(failure)"
            lastDiagnostics = diag
            throw failure
        }

        let text = await box.waitForFinal(timeout: Self.finalResultTimeout)
        tearDown()
        diag.transcriptLength = text.count
        if text.isEmpty { diag.failure = "emptyTranscript" }
        lastDiagnostics = diag
        log("transcribe result chars=\(text.count)") // 話した内容はログに出さない
        return text
    }

    nonisolated static func precheck(frames: Int, seconds: Double, peak: Float) -> TranscriberFailure? {
        if frames == 0 { return .noAudioCaptured(seconds: seconds) }
        if seconds < minimumSeconds { return .tooShort(seconds: seconds) }
        if peak < silencePeak { return .silent(seconds: seconds, peakDecibels: 20 * log10(max(peak, 1e-6))) }
        return nil
    }

    private func tearDown() {
        engine?.stop()
        engine?.inputNode.removeTap(onBus: 0)
        engine = nil
        request = nil
        task = nil
        box = nil
        meter = nil
        recordingStartedAt = nil
        isRecording = false
    }

    // 音声のスレッドから呼ばれるので MainActor の外で作る
    nonisolated private static func makeTap(request: SFSpeechAudioBufferRecognitionRequest, meter: AudioMeter) -> AVAudioNodeTapBlock {
        { buffer, _ in
            request.append(buffer)
            meter.add(buffer)
        }
    }

    nonisolated private static func makeResultHandler(box: RecognitionBox) -> (SFSpeechRecognitionResult?, Error?) -> Void {
        { result, error in
            if let result {
                box.update(text: result.bestTranscription.formattedString, isFinal: result.isFinal)
            }
            if error != nil { box.finish() }
        }
    }
}

/// 入ってきた音の量(フレーム数・ピーク)。音声のスレッドから足す。
private final class AudioMeter: @unchecked Sendable {
    private let lock = NSLock()
    private var frames = 0
    private var peak: Float = 0

    func add(_ buffer: AVAudioPCMBuffer) {
        let n = Int(buffer.frameLength)
        var p: Float = 0
        if let data = buffer.floatChannelData?[0] {
            for i in 0..<n { p = max(p, abs(data[i])) }
        }
        lock.lock()
        frames += n
        peak = max(peak, p)
        lock.unlock()
    }

    func snapshot() -> (frames: Int, peak: Float) {
        lock.lock(); defer { lock.unlock() }
        return (frames, peak)
    }
}

/// 認識の結果を 1 回だけ返す入れ物。途中の結果を覚えておき、時間切れ・エラーではそれを返す。
private final class RecognitionBox: @unchecked Sendable {
    private let lock = NSLock()
    private var latest = ""
    private var done = false
    private var waiter: CheckedContinuation<String, Never>?

    func update(text: String, isFinal: Bool) {
        lock.lock()
        latest = text
        lock.unlock()
        if isFinal { finish() }
    }

    func finish() {
        lock.lock()
        done = true
        let w = waiter
        waiter = nil
        let text = latest
        lock.unlock()
        w?.resume(returning: text.trimmingCharacters(in: .whitespacesAndNewlines))
    }

    func waitForFinal(timeout: Duration) async -> String {
        let timer = Task { [weak self] in
            try? await Task.sleep(for: timeout)
            self?.finish()
        }
        defer { timer.cancel() }
        return await withCheckedContinuation { cont in
            lock.lock()
            if done {
                let text = latest
                lock.unlock()
                cont.resume(returning: text.trimmingCharacters(in: .whitespacesAndNewlines))
            } else {
                waiter = cont
                lock.unlock()
            }
        }
    }
}
