import SwiftUI
import AVFoundation
import MetamonDSL

/// audio field の再生(#420)。アプリ全体で同時に鳴らすのは 1 本だけ(一覧の別の行を押したら前のを止める)。
@MainActor @Observable
final class AudioPlayback: NSObject, AVAudioPlayerDelegate {
    static let shared = AudioPlayback()

    /// 再生中の録音の id(nil = 止まっている)
    private(set) var playingID: String?
    /// 再生位置(秒)。0.2 秒ごとに更新
    private(set) var position: Double = 0

    @ObservationIgnored private var player: AVAudioPlayer?
    @ObservationIgnored private var lease: AudioSessionLease?
    @ObservationIgnored private var ticker: Task<Void, Never>?

    func isPlaying(_ id: String?) -> Bool { id != nil && playingID == id }

    /// 押した録音が鳴っていれば止め、そうでなければ(前のを止めて)鳴らす。ファイルが無ければ false。
    @discardableResult
    func toggle(_ value: AudioValue, store: LocalAudioStore = .shared) -> Bool {
        if playingID == value.id {
            stop()
            return true
        }
        stop()
        guard store.exists(value.id), let url = store.url(for: value.id) else { return false }
        do {
            let lease = try AudioSessionLease.forPlayback()
            let p = try AVAudioPlayer(contentsOf: url)
            p.delegate = self
            guard p.play() else {
                lease.end()
                return false
            }
            self.lease = lease
            player = p
            playingID = value.id
            position = 0
            ticker = Task { [weak self] in
                while !Task.isCancelled {
                    try? await Task.sleep(for: .milliseconds(200))
                    guard let self, let p = self.player else { return }
                    self.position = p.currentTime
                }
            }
            return true
        } catch {
            print("[Metamon] audio playback failed: \(error)")
            return false
        }
    }

    func stop() {
        ticker?.cancel()
        ticker = nil
        player?.stop()
        player = nil
        playingID = nil
        position = 0
        lease?.end()
        lease = nil
    }

    nonisolated func audioPlayerDidFinishPlaying(_ player: AVAudioPlayer, successfully flag: Bool) {
        Task { @MainActor in self.stop() }
    }

    nonisolated func audioPlayerDecodeErrorDidOccur(_ player: AVAudioPlayer, error: Error?) {
        Task { @MainActor in self.stop() }
    }
}

/// 一覧の行の右端(`list.item.trailing` が audio field・#420): 再生 / 停止ボタン + 長さ。
/// この端末に録音が無ければ(他の端末で録った)長さだけを薄く出す。
struct AudioPlayButton: View {
    let value: JSONValue
    var store: LocalAudioStore = .shared
    private var playback: AudioPlayback { .shared }

    var body: some View {
        if let audio = AudioValue(value) {
            let playing = playback.isPlaying(audio.id)
            let available = store.exists(audio.id)
            Button {
                playback.toggle(audio, store: store)
            } label: {
                HStack(spacing: 6) {
                    Image(systemName: playing ? "stop.circle.fill" : "play.circle.fill")
                        .font(.title2)
                        .foregroundStyle(available ? RenderTheme.Colors.brandA : Color.secondary)
                        .contentTransition(.symbolEffect(.replace))
                    Text(AudioRecorderPolicy.elapsedText(Double(playing ? Int(playback.position) : audio.seconds)))
                        .font(.subheadline.monospacedDigit())
                        .foregroundStyle(.secondary)
                }
                .padding(8)
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .padding(-8)
            .disabled(!available)
            .accessibilityLabel(playing ? L("停止") : L("録音を再生"))
        }
    }
}
