import Foundation

/// Mac の「録音と再生は同時に 1 つだけ」の約束(#952)。iOS は AVAudioSession(`AudioSessionLease`)が取り合いを決めるが、Mac には無い。
/// - 録音を始めたら、再生(audio field)と読み上げ(speakButton・ホストの読み上げ)を止める
/// - 録音している間(部品の録音・ホストの声の入力)は、再生を始めない・別の録音も始めない
/// Mac の `AudioSessionLease` がこれを使う。ホストの声の入力(声で頼む・話して入力)は `isMicBusyElsewhere` でつなぐ。
@MainActor
public final class AudioFocus {
    public static let shared = AudioFocus(stopPlayback: {
        AudioPlayback.shared.stop()
        LeafSpeech.shared.stop()
    })

    public enum Kind: Equatable { case recording, playback }

    public enum Refusal: Error, Equatable {
        /// 録音中なので再生できない
        case recordingInProgress
        /// ほかの録音(部品・声の入力)がマイクを使っている
        case micInUse
    }

    /// ホストの声の入力がマイクを使っているか(Mac: 声で頼む・話して入力)
    public var isMicBusyElsewhere: () -> Bool = { false }
    /// 録音を始める時に、ホストが止める再生(Mac: 完成の要約の読み上げ)
    public var stopHostPlayback: () -> Void = {}

    private let stopPlayback: () -> Void
    private var held: [Int: Kind] = [:]
    private var nextToken = 0

    init(stopPlayback: @escaping () -> Void) {
        self.stopPlayback = stopPlayback
    }

    /// 部品が録音しているか
    public var isRecording: Bool { held.values.contains(.recording) }
    public var isPlaying: Bool { held.values.contains(.playback) }
    /// 部品かホストの声の入力が録音しているか
    public var isRecordingAnywhere: Bool { isRecording || isMicBusyElsewhere() }

    /// 録音を始める: 録音中なら断り、鳴っている再生・読み上げを止める。返した印を `end` に渡す。
    func beginRecording() throws -> Int {
        guard !isRecordingAnywhere else { throw Refusal.micInUse }
        silence()
        return hold(.recording)
    }

    /// 再生を始める: 録音中なら断る。
    func beginPlayback() throws -> Int {
        guard !isRecordingAnywhere else { throw Refusal.recordingInProgress }
        return hold(.playback)
    }

    /// 借りた分を返す。何度呼んでもよい。
    func end(_ token: Int) {
        held[token] = nil
    }

    /// ホストの声の入力が録音を始める時: 再生・読み上げを止める(マイクが拾わない)。
    public func micWillStart() {
        silence()
    }

    private func silence() {
        stopPlayback()
        stopHostPlayback()
        held = held.filter { $0.value != .playback }
    }

    private func hold(_ kind: Kind) -> Int {
        nextToken += 1
        held[nextToken] = kind
        return nextToken
    }
}
