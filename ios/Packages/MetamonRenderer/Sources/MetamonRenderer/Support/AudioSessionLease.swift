import AVFoundation

/// AVAudioSession のうち、録音・再生の部品が触る所だけ(テストで差し替える)。
protocol AudioSessionControlling: AnyObject {
    var category: AVAudioSession.Category { get }
    var mode: AVAudioSession.Mode { get }
    var categoryOptions: AVAudioSession.CategoryOptions { get }
    func setCategory(_ category: AVAudioSession.Category, mode: AVAudioSession.Mode, options: AVAudioSession.CategoryOptions) throws
    func setActive(_ active: Bool, options: AVAudioSession.SetActiveOptions) throws
}

extension AVAudioSession: AudioSessionControlling {}

/// 録音 / 再生の間だけ共有セッションを借り、終わったら借りる前の category / mode / options に戻す(#420)。
/// アプリの音声入力(`WhisperKitRecognizer` は開始のたびに `.playAndRecord` を自分で設定する)と
/// 読み上げ(`SpeechReader` は別セッション)を壊さないため。戻した後は非アクティブにして他のアプリの音を戻す。
struct AudioSessionLease {
    struct Snapshot: Equatable {
        var category: AVAudioSession.Category
        var mode: AVAudioSession.Mode
        var options: AVAudioSession.CategoryOptions
    }

    let saved: Snapshot
    let applied: Snapshot
    private let session: AudioSessionControlling

    /// 録音: 内蔵スピーカー + Bluetooth(AirPods)のマイクも使える(音声入力と同じ設定)。
    static func forRecording(_ session: AudioSessionControlling = AVAudioSession.sharedInstance()) throws -> AudioSessionLease {
        try AudioSessionLease(session, category: .playAndRecord, mode: .default, options: [.defaultToSpeaker, .allowBluetoothHFP, .allowBluetoothA2DP])
    }

    /// 再生: マナーモードでも鳴る(ユーザーが再生ボタンを押した時だけ)。
    static func forPlayback(_ session: AudioSessionControlling = AVAudioSession.sharedInstance()) throws -> AudioSessionLease {
        try AudioSessionLease(session, category: .playback, mode: .default, options: [])
    }

    init(_ session: AudioSessionControlling, category: AVAudioSession.Category, mode: AVAudioSession.Mode, options: AVAudioSession.CategoryOptions) throws {
        self.session = session
        saved = Snapshot(category: session.category, mode: session.mode, options: session.categoryOptions)
        applied = Snapshot(category: category, mode: mode, options: options)
        try session.setCategory(category, mode: mode, options: options)
        do {
            try session.setActive(true, options: [])
        } catch {
            try? session.setCategory(saved.category, mode: saved.mode, options: saved.options)
            throw error
        }
    }

    /// 借りる前に戻す。何度呼んでもよい。
    /// 途中で他(音声入力の録音開始など)が設定を変えていたら、その人の設定を壊さないよう何もしない。
    func end() {
        guard Snapshot(category: session.category, mode: session.mode, options: session.categoryOptions) == applied else { return }
        try? session.setActive(false, options: .notifyOthersOnDeactivation)
        try? session.setCategory(saved.category, mode: saved.mode, options: saved.options)
    }
}
