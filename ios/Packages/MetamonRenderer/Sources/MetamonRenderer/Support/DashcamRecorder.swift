// 審査用ビルド(APP_STORE_REVIEW・#245)ではバイナリから外す(カメラでの撮影・CoreMotion)。
#if !APP_STORE_REVIEW
import Foundation
import AVFoundation
import CoreMotion
import Observation
import Photos
import UIKit
import MetamonDSL

/// `dashcam`(#410)の端末内の置き場: Application Support/metamon-dashcam/<アプリ名のハッシュ>/{loop,saved}/。
/// 映像はレコードに入れない(同期しない・iCloud バックアップの対象外)。
struct DashcamStore: Sendable {
    let root: URL

    init(appName: String, base: URL? = nil) {
        let b = base ?? Self.baseDirectory
        root = b.appendingPathComponent(DashcamStoreKey.make(appName: appName), isDirectory: true)
    }

    static var baseDirectory: URL {
        (FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first ?? FileManager.default.temporaryDirectory)
            .appendingPathComponent("metamon-dashcam", isDirectory: true)
    }

    var loopDir: URL { root.appendingPathComponent("loop", isDirectory: true) }
    var savedDir: URL { root.appendingPathComponent("saved", isDirectory: true) }

    func prepare() {
        for d in [loopDir, savedDir] {
            try? FileManager.default.createDirectory(at: d, withIntermediateDirectories: true)
        }
        var r = root
        var v = URLResourceValues()
        v.isExcludedFromBackup = true
        try? r.setResourceValues(v)
        // 落ちた等で閉じられなかった録画中のファイル(再生できない)を消す
        for u in (try? FileManager.default.contentsOfDirectory(at: loopDir, includingPropertiesForKeys: nil)) ?? []
        where u.lastPathComponent.hasPrefix("rec-") {
            try? FileManager.default.removeItem(at: u)
        }
    }

    func url(for s: DashcamSegment) -> URL {
        (s.saved ? savedDir : loopDir).appendingPathComponent(s.fileName)
    }

    func recordingURL(start: Date) -> URL {
        loopDir.appendingPathComponent(DashcamSegment.recordingName(start: start))
    }

    /// 両方の置き場の区間(古い順)。
    func segments() -> [DashcamSegment] {
        var out: [DashcamSegment] = []
        for (dir, saved) in [(loopDir, false), (savedDir, true)] {
            let urls = (try? FileManager.default.contentsOfDirectory(at: dir, includingPropertiesForKeys: [.fileSizeKey])) ?? []
            for u in urls {
                let size = (try? u.resourceValues(forKeys: [.fileSizeKey]).fileSize).flatMap { $0 } ?? 0
                if let s = DashcamSegment(fileName: u.lastPathComponent, bytes: Int64(size), saved: saved) { out.append(s) }
            }
        }
        return out.sorted { $0.start < $1.start }
    }

    /// 録画中のファイルを区間として確定する(長さをファイル名に入れる)。
    func finalize(_ recording: URL, start: Date, duration: TimeInterval, saved: Bool) -> DashcamSegment? {
        guard FileManager.default.fileExists(atPath: recording.path) else { return nil }
        let size = (try? recording.resourceValues(forKeys: [.fileSizeKey]).fileSize).flatMap { $0 } ?? 0
        let s = DashcamSegment(start: start, duration: max(0, duration), bytes: Int64(size), saved: saved)
        do {
            try FileManager.default.moveItem(at: recording, to: url(for: s))
            return s
        } catch {
            try? FileManager.default.removeItem(at: recording)
            return nil
        }
    }

    /// ループの区間を保存済みに移す。
    @discardableResult
    func save(_ s: DashcamSegment) -> DashcamSegment? {
        guard !s.saved else { return s }
        var moved = s
        moved.saved = true
        do {
            try FileManager.default.moveItem(at: url(for: s), to: url(for: moved))
            return moved
        } catch { return nil }
    }

    func delete(_ s: DashcamSegment) {
        try? FileManager.default.removeItem(at: url(for: s))
    }

    /// 端末の空き容量(取れなければ nil)。
    func deviceFreeBytes() -> Int64? {
        try? root.resourceValues(forKeys: [.volumeAvailableCapacityForImportantUsageKey]).volumeAvailableCapacityForImportantUsage
    }
}

/// AVCaptureSession と録画の出力。セッションの操作は専用の直列キューで行う(main を止めない)。
final class DashcamCapture: NSObject, AVCaptureFileOutputRecordingDelegate, @unchecked Sendable {
    let session = AVCaptureSession()
    private let output = AVCaptureMovieFileOutput()
    private let queue = DispatchQueue(label: "metamon.dashcam.session")
    private(set) var device: AVCaptureDevice?
    /// 1 区間の録画が終わった(エラーでもファイルがあれば中身は使える)。main で呼ぶ。
    var onFinished: (@MainActor (URL) -> Void)?

    static var hasBackCamera: Bool {
        AVCaptureDevice.default(.builtInWideAngleCamera, for: .video, position: .back) != nil
    }

    /// 入出力を組む。カメラが無い・入力を足せなければ false。
    func configure(audio: Bool) async -> Bool {
        await withCheckedContinuation { cont in
            queue.async { [self] in
                cont.resume(returning: configureOnQueue(audio: audio))
            }
        }
    }

    private func configureOnQueue(audio: Bool) -> Bool {
        if device != nil { return true }
        guard let cam = AVCaptureDevice.default(.builtInWideAngleCamera, for: .video, position: .back),
              let input = try? AVCaptureDeviceInput(device: cam) else { return false }
        session.beginConfiguration()
        defer { session.commitConfiguration() }
        // 720p: 1 分 ≈ 40〜60MB(容量の上限・ループの見込みの前提)
        if session.canSetSessionPreset(.hd1280x720) { session.sessionPreset = .hd1280x720 }
        guard session.canAddInput(input) else { return false }
        session.addInput(input)
        if audio, let mic = AVCaptureDevice.default(for: .audio), let micInput = try? AVCaptureDeviceInput(device: mic), session.canAddInput(micInput) {
            session.addInput(micInput)
        }
        guard session.canAddOutput(output) else { return false }
        session.addOutput(output)
        device = cam
        return true
    }

    func startRunning() { queue.async { [session] in if !session.isRunning { session.startRunning() } } }
    func stopRunning() { queue.async { [session] in if session.isRunning { session.stopRunning() } } }

    /// 始められなければ(セッションが止まっている等)ファイル無しで `onFinished` を呼ぶ。
    func startSegment(to url: URL, rotation: CGFloat) {
        queue.async { [self] in
            guard session.isRunning, !output.isRecording, let conn = output.connection(with: .video) else {
                Task { @MainActor [weak self] in self?.onFinished?(url) }
                return
            }
            if conn.isVideoRotationAngleSupported(rotation) { conn.videoRotationAngle = rotation }
            output.startRecording(to: url, recordingDelegate: self)
        }
    }

    func stopSegment() {
        queue.async { [output] in if output.isRecording { output.stopRecording() } }
    }

    func fileOutput(_ output: AVCaptureFileOutput, didFinishRecordingTo outputFileURL: URL, from connections: [AVCaptureConnection], error: Error?) {
        Task { @MainActor [weak self] in self?.onFinished?(outputFileURL) }
    }
}

/// ドラレコの状態。部品が表示されている間だけ動く(画面を離れる・裏に回ると止める)。
@MainActor
@Observable
final class DashcamRecorder {
    enum Phase: Equatable {
        case idle, preparing, ready, recording
        /// カメラが無い(シミュレータ等)
        case unavailable
        /// カメラ(とマイク)の許可が無い
        case denied
        /// 保存済みだけで容量の上限 / 端末の空きが少ない
        case full(deviceLow: Bool)
    }

    let config: DashcamConfig
    let store: DashcamStore
    private(set) var phase: Phase = .idle
    private(set) var recordingSince: Date?
    /// 保存済みの区間(新しい順)
    private(set) var clips: [DashcamSegment] = []
    /// ループに残っている区間の合計秒
    private(set) var loopSeconds: TimeInterval = 0
    private(set) var lastImpact: Date?
    private(set) var autoStartIn: TimeInterval?
    /// 止まった理由など、利用者に伝えること
    private(set) var notice: String?
    /// 衝撃・手動の保存の回数(触覚フィードバックのきっかけ)
    private(set) var impactCount = 0

    @ObservationIgnored let capture = DashcamCapture()
    @ObservationIgnored private let motion = CMMotionManager()
    @ObservationIgnored private var impact: DashcamImpact
    @ObservationIgnored private var autoStart: DashcamAutoStart
    /// 録画中・閉じている途中の区間のファイルと開始時刻
    @ObservationIgnored private var starts: [URL: Date] = [:]
    @ObservationIgnored private var active: URL?
    @ObservationIgnored private var rotateTask: Task<Void, Never>?
    @ObservationIgnored private var rotation: AVCaptureDevice.RotationCoordinator?
    @ObservationIgnored private var wantsRecording = false
    @ObservationIgnored private var resumeAfterBackground = false

    /// 端末の空きがこれを切ったら録画を止める
    static let minimumFreeBytes: Int64 = 500_000_000

    init(config: DashcamConfig, store: DashcamStore) {
        self.config = config
        self.store = store
        impact = DashcamImpact(thresholdG: config.impactG)
        autoStart = DashcamAutoStart(conditions: config.autoStart)
        capture.onFinished = { [weak self] url in self?.segmentFinished(url) }
    }

    var isRecording: Bool { phase == .recording }

    // MARK: - lifecycle(部品の表示・裏/表)

    func appear() async {
        store.prepare()
        refresh()
        guard phase == .idle else { return }
        guard DashcamCapture.hasBackCamera else {
            phase = .unavailable
            return
        }
        phase = .preparing
        guard await Self.authorize(audio: config.audio) else {
            phase = .denied
            return
        }
        guard await capture.configure(audio: config.audio) else {
            phase = .unavailable
            return
        }
        if let device = capture.device { rotation = AVCaptureDevice.RotationCoordinator(device: device, previewLayer: nil) }
        capture.startRunning()
        phase = .ready
        startMotion()
    }

    func disappear() {
        stop(manual: false)
        motion.stopDeviceMotionUpdates()
        capture.stopRunning()
        if phase != .unavailable && phase != .denied { phase = .idle }
        UIDevice.current.isBatteryMonitoringEnabled = false
    }

    /// 裏に回った: iOS はカメラを止めるので、区間を閉じて止める。戻ったら続ける。
    func enterBackground() {
        guard phase == .recording || phase == .ready else { return }
        resumeAfterBackground = phase == .recording
        if resumeAfterBackground { notice = L("画面を閉じたので録画を止めました。画面を出している間だけ録画できます") }
        stop(manual: false)
        capture.stopRunning()
    }

    func enterForeground() {
        guard phase == .ready else { return }
        capture.startRunning()
        autoStart.rearm()
        if resumeAfterBackground {
            resumeAfterBackground = false
            start()
        }
    }

    // MARK: - recording

    func start() {
        guard phase == .ready else { return }
        if let low = storageProblem() {
            phase = .full(deviceLow: low)
            return
        }
        notice = nil
        wantsRecording = true
        autoStart.disarm()
        phase = .recording
        recordingSince = .now
        UIApplication.shared.isIdleTimerDisabled = true
        if active == nil { beginSegment() } // 前の区間を閉じている途中なら、閉じた後に始まる
    }

    /// manual = 利用者が止めた(自動開始は条件が一度外れるまで待つ)。
    func stop(manual: Bool = true) {
        if manual { autoStart.disarm() }
        guard wantsRecording else { return }
        wantsRecording = false
        rotateTask?.cancel()
        capture.stopSegment()
        if phase == .recording { phase = .ready }
        recordingSince = nil
        UIApplication.shared.isIdleTimerDisabled = false
    }

    /// 「今の映像を保存」: 衝撃が無くても前後を保存済みにする。
    func saveNow() {
        mark(at: .now)
    }

    func delete(_ clip: DashcamSegment) {
        store.delete(clip)
        refresh()
        if case .full = phase, storageProblem() == nil { phase = .ready }
    }

    func url(for clip: DashcamSegment) -> URL { store.url(for: clip) }

    /// 写真ライブラリに追加する(追加だけの許可)。
    func saveToPhotos(_ clip: DashcamSegment) async -> Bool {
        let status = await PHPhotoLibrary.requestAuthorization(for: .addOnly)
        guard status == .authorized || status == .limited else { return false }
        let url = store.url(for: clip)
        do {
            try await PHPhotoLibrary.shared().performChanges {
                _ = PHAssetCreationRequest.creationRequestForAssetFromVideo(atFileURL: url)
            }
            return true
        } catch {
            return false
        }
    }

    private func beginSegment() {
        guard active == nil else { return }
        let start = Date.now
        let url = store.recordingURL(start: start)
        starts[url] = start
        active = url
        capture.startSegment(to: url, rotation: rotation?.videoRotationAngleForHorizonLevelCapture ?? 90)
        rotateTask?.cancel()
        rotateTask = Task { [weak self] in
            try? await Task.sleep(for: .seconds(DashcamConfig.segmentSeconds))
            guard !Task.isCancelled else { return }
            self?.capture.stopSegment()
        }
    }

    private func segmentFinished(_ url: URL) {
        let start = starts.removeValue(forKey: url) ?? .now
        if active == url { active = nil }
        let end = Date.now
        let saved = impact.protects(start: start, end: end)
        let kept = store.finalize(url, start: start, duration: end.timeIntervalSince(start), saved: saved)
        impact.forget(before: end.addingTimeInterval(-DashcamImpact.before - DashcamConfig.segmentSeconds))
        prune()
        guard wantsRecording else { return }
        if let low = storageProblem() {
            stop(manual: false)
            phase = .full(deviceLow: low)
            return
        }
        guard kept == nil else { return beginSegment() }
        // 録れなかった(電話などでカメラが中断中): 少し待ってやり直す
        rotateTask?.cancel()
        rotateTask = Task { [weak self] in
            try? await Task.sleep(for: .seconds(1))
            guard !Task.isCancelled, self?.wantsRecording == true else { return }
            self?.beginSegment()
        }
    }

    /// ループの古い区間を消して一覧を読み直す。
    private func prune() {
        for s in DashcamLoop.expired(store.segments(), now: .now, keepSeconds: config.keepSeconds, maxBytes: config.maxBytes) {
            store.delete(s)
        }
        refresh()
    }

    private func refresh() {
        let all = store.segments()
        clips = all.filter(\.saved).sorted { $0.start > $1.start }
        loopSeconds = all.filter { !$0.saved }.reduce(0) { $0 + $1.duration }
    }

    /// 録れない理由(nil = 録れる)。true = 端末の空きが少ない / false = 保存済みで上限。
    private func storageProblem() -> Bool? {
        if let free = store.deviceFreeBytes(), free < Self.minimumFreeBytes { return true }
        let all = store.segments()
        return DashcamLoop.isFull(all, maxBytes: config.maxBytes, reserve: DashcamLoop.reserve(all)) ? false : nil
    }

    private func mark(at date: Date) {
        impact.mark(at: date)
        protectFinished()
        lastImpact = date
        impactCount += 1
    }

    /// 既に閉じたループ区間で衝撃の前後にかかるものを保存済みに移す(録画中の区間は閉じた時に判定)。
    private func protectFinished() {
        for s in store.segments() where !s.saved && impact.protects(start: s.start, end: s.end) {
            store.save(s)
        }
        refresh()
    }

    // MARK: - motion(衝撃・自動開始)

    private func startMotion() {
        UIDevice.current.isBatteryMonitoringEnabled = true
        guard motion.isDeviceMotionAvailable, !motion.isDeviceMotionActive else { return }
        motion.deviceMotionUpdateInterval = 1.0 / 20
        motion.startDeviceMotionUpdates(to: .main) { [weak self] m, _ in
            guard let m else { return }
            MainActor.assumeIsolated { self?.handle(m) }
        }
    }

    private func handle(_ m: CMDeviceMotion) {
        let now = Date.now
        let a = m.userAcceleration
        if phase == .recording, impact.feed(x: a.x, y: a.y, z: a.z, at: now) {
            protectFinished()
            lastImpact = now
            impactCount += 1
        }
        guard !config.autoStart.isEmpty else { return }
        let r = m.rotationRate
        let state = UIDevice.current.batteryState
        let sample = DashcamAutoStart.Sample(
            gravity: (m.gravity.x, m.gravity.y, m.gravity.z),
            acceleration: (a.x * a.x + a.y * a.y + a.z * a.z).squareRoot(),
            rotation: (r.x * r.x + r.y * r.y + r.z * r.z).squareRoot(),
            charging: state == .charging || state == .full,
            at: now)
        let go = autoStart.feed(sample)
        let left = phase == .ready ? autoStart.remaining(at: now).map { $0.rounded(.up) } : nil
        if left != autoStartIn { autoStartIn = left }
        if go, phase == .ready { start() }
    }

    // MARK: - permission

    private static func authorize(audio: Bool) async -> Bool {
        guard await access(.video) else { return false }
        return audio ? await access(.audio) : true
    }

    private static func access(_ media: AVMediaType) async -> Bool {
        switch AVCaptureDevice.authorizationStatus(for: media) {
        case .authorized: true
        case .notDetermined: await AVCaptureDevice.requestAccess(for: media)
        default: false
        }
    }
}
#endif
