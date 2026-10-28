#if !APP_STORE_REVIEW
// 審査用ビルド(APP_STORE_REVIEW・#245)では録音の部品ごと外す(4.7.2: DSL からマイクを使わせない)。
import AVFoundation
import MediaPlayer
import MetamonDSL

/// `audioRecorder`(#420)の録音。AVAudioRecorder で AAC(m4a・モノラル 64kbps)を一時ファイルに録り、
/// 止めたら `LocalAudioStore` に取り込んで `onRecorded` で値を返す。上限は `record(forDuration:)` が止める。
@MainActor @Observable
final class AudioRecorderController: NSObject, AVAudioRecorderDelegate {
    enum Failure: Equatable { case denied, failed }

    private(set) var machine: AudioRecorderMachine
    /// 録音中の経過秒・音量(0...1)。0.1 秒ごとに更新
    private(set) var elapsed: Double = 0
    private(set) var level: Double = 0
    private(set) var failure: Failure?
    let maxDuration: Int

    @ObservationIgnored var onRecorded: ((AudioValue) -> Void)?
    @ObservationIgnored private let store: LocalAudioStore
    @ObservationIgnored private var recorder: AVAudioRecorder?
    @ObservationIgnored private var lease: AudioSessionLease?
    @ObservationIgnored private var ticker: Task<Void, Never>?
    @ObservationIgnored private var interruption: NSObjectProtocol?

    static let settings: [String: Any] = [
        AVFormatIDKey: kAudioFormatMPEG4AAC,
        AVSampleRateKey: 44_100,
        AVNumberOfChannelsKey: 1,
        AVEncoderBitRateKey: 64_000,
        AVEncoderAudioQualityKey: AVAudioQuality.high.rawValue,
    ]

    init(maxDuration: Int?, hasClip: Bool, store: LocalAudioStore = .shared) {
        self.maxDuration = AudioRecorderPolicy.maxDuration(maxDuration)
        self.machine = AudioRecorderMachine(hasClip: hasClip)
        self.store = store
    }

    var isRecording: Bool { machine.phase == .recording }

    /// フォームの値が外から変わった(削除・別レコード)時に合わせる。録音中は触らない。
    func sync(hasClip: Bool) {
        guard !isRecording else { return }
        machine = AudioRecorderMachine(hasClip: hasClip)
    }

    func start() async {
        guard !isRecording else { return }
        failure = nil
        AudioPlayback.shared.stop()
        guard await AVAudioApplication.requestRecordPermission() else {
            failure = .denied
            return
        }
        guard machine.send(.start) else { return }
        do {
            let lease = try AudioSessionLease.forRecording()
            let url = LocalAudioStore.temporaryRecordingURL()
            let r = try AVAudioRecorder(url: url, settings: Self.settings)
            r.delegate = self
            r.isMeteringEnabled = true
            guard r.record(forDuration: TimeInterval(maxDuration)) else {
                lease.end()
                throw CocoaError(.fileWriteUnknown)
            }
            self.lease = lease
            recorder = r
            elapsed = 0
            level = 0
            startTicker()
            observeInterruptions()
        } catch {
            print("[Metamon] audio record failed: \(error)")
            machine.send(.fail)
            failure = .failed
        }
    }

    /// 止める(録れた分は残す)。結果は delegate 経由で `onRecorded` に届く。
    func stop() {
        recorder?.stop()
    }

    func discard() {
        AudioPlayback.shared.stop()
        machine.send(.discard)
    }

    nonisolated func audioRecorderDidFinishRecording(_ recorder: AVAudioRecorder, successfully flag: Bool) {
        let url = recorder.url
        Task { @MainActor in self.finish(url: url, success: flag) }
    }

    nonisolated func audioRecorderEncodeErrorDidOccur(_ recorder: AVAudioRecorder, error: Error?) {
        let url = recorder.url
        Task { @MainActor in self.finish(url: url, success: false) }
    }

    private func finish(url: URL, success: Bool) {
        guard isRecording else { return }
        ticker?.cancel()
        ticker = nil
        if let interruption { NotificationCenter.default.removeObserver(interruption) }
        interruption = nil
        recorder = nil
        lease?.end()
        lease = nil
        level = 0
        let duration = (try? AVAudioPlayer(contentsOf: url))?.duration ?? elapsed
        if success, duration > 0, let id = try? store.importFile(at: url) {
            machine.send(.stop)
            onRecorded?(AudioValue(id: id, seconds: AudioRecorderPolicy.storedSeconds(duration)))
        } else {
            try? FileManager.default.removeItem(at: url)
            machine.send(.fail)
            failure = .failed
        }
    }

    private func startTicker() {
        ticker = Task { [weak self] in
            while !Task.isCancelled {
                try? await Task.sleep(for: .milliseconds(100))
                guard let self, let r = self.recorder else { return }
                r.updateMeters()
                self.elapsed = r.currentTime
                self.level = AudioRecorderPolicy.meterLevel(power: r.averagePower(forChannel: 0))
            }
        }
    }

    /// 電話・Siri などで中断されたら、そこまでを残して止める。
    private func observeInterruptions() {
        interruption = NotificationCenter.default.addObserver(forName: AVAudioSession.interruptionNotification, object: nil, queue: .main) { [weak self] note in
            guard let raw = note.userInfo?[AVAudioSessionInterruptionTypeKey] as? UInt,
                  AVAudioSession.InterruptionType(rawValue: raw) == .began else { return }
            MainActor.assumeIsolated { self?.stop() }
        }
    }
}

/// イヤホン(AirPods など)の 再生 / 一時停止 ボタンを録音部品に渡す(#420)。
/// 録音部品が画面にある間だけ登録し、離れたら外す(同時に 1 つ・後から出た部品が持つ)。
/// iOS がボタンを届けるのは「最後に音を扱ったアプリ」だけなので、他のアプリが音楽を流していると届かない。
@MainActor
final class RemoteRecorderCommands {
    static let shared = RemoteRecorderCommands()

    private var owner: ObjectIdentifier?
    private var handler: ((AudioRecorderMachine.RemoteCommand) -> Void)?
    private var targets: [(MPRemoteCommand, Any)] = []

    var isAttached: Bool { owner != nil }

    func attach(owner: AnyObject, title: String, handler: @escaping (AudioRecorderMachine.RemoteCommand) -> Void) {
        detachAll()
        self.owner = ObjectIdentifier(owner)
        self.handler = handler
        let center = MPRemoteCommandCenter.shared()
        for (command, kind) in [(center.togglePlayPauseCommand, AudioRecorderMachine.RemoteCommand.togglePlayPause),
                                (center.playCommand, .play), (center.pauseCommand, .pause)] {
            command.isEnabled = true
            let target = command.addTarget { [weak self] _ in
                MainActor.assumeIsolated { self?.handler?(kind) }
                return .success
            }
            targets.append((command, target))
        }
        MPNowPlayingInfoCenter.default().nowPlayingInfo = [MPMediaItemPropertyTitle: title]
    }

    /// その部品が今の持ち主の時だけ外す(画面遷移で新しい部品が先に attach しても消さない)。
    func detach(owner: AnyObject) {
        guard self.owner == ObjectIdentifier(owner) else { return }
        detachAll()
    }

    private func detachAll() {
        for (command, target) in targets { command.removeTarget(target) }
        targets = []
        handler = nil
        owner = nil
        MPNowPlayingInfoCenter.default().nowPlayingInfo = nil
    }
}
#endif
