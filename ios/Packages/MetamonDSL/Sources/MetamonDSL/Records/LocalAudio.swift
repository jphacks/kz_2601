import Foundation

/// audio field の値(#420)。録音本体(AAC m4a)は録音した端末のアプリ領域にだけ置き、
/// レコードには `"audio:<id>|<秒>"` だけを入れる(video #206 と同じ方針・Firestore 1MB 制限)。
public struct AudioValue: Equatable, Sendable {
    /// 端末内のファイル id(`LocalAudioStore` のファイル名)
    public var id: String
    /// 長さ(秒・切り上げ)。0 = 不明
    public var seconds: Int

    public init(id: String, seconds: Int) {
        self.id = id
        self.seconds = max(0, seconds)
    }

    public static let prefix = "audio:"

    public init?(_ value: JSONValue) {
        guard let s = value.stringValue else { return nil }
        self.init(string: s)
    }

    public init?(string s: String) {
        guard s.hasPrefix(Self.prefix) else { return nil }
        let parts = s.dropFirst(Self.prefix.count).split(separator: "|", maxSplits: 1, omittingEmptySubsequences: false)
        let id = String(parts[0])
        guard LocalAudioStore.isValidID(id) else { return nil }
        self.init(id: id, seconds: parts.count > 1 ? Int(parts[1]) ?? 0 : 0)
    }

    /// レコードに保存する文字列。
    public var encoded: String { Self.prefix + id + "|\(seconds)" }

    /// レコードの値の中にある端末内録音の id(削除時に一緒にファイルを消す)。
    public static func localIDs(in data: [String: JSONValue]) -> Set<String> {
        Set(data.values.compactMap { AudioValue($0)?.id })
    }
}

/// 端末内の録音ファイル置き場(Application Support/metamon-audio/<id>.m4a)。
/// レコード削除・アプリ削除・アカウント削除の時に `RecordStore` / `RecordSyncEngine` から消す。
public struct LocalAudioStore: Sendable {
    public let directory: URL

    public init(directory: URL? = nil) {
        let base = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first
            ?? FileManager.default.temporaryDirectory
        self.directory = directory ?? base.appendingPathComponent("metamon-audio", isDirectory: true)
    }

    public static let shared = LocalAudioStore()
    public static let fileExtension = "m4a"

    /// id は英数・-・_ だけ(パスを組み立てるので `..` や `/` を通さない)。
    public static func isValidID(_ id: String) -> Bool { LocalVideoStore.isValidID(id) }

    public static func newID() -> String { UUID().uuidString }

    public func url(for id: String) -> URL? {
        guard Self.isValidID(id) else { return nil }
        return directory.appendingPathComponent("\(id).\(Self.fileExtension)")
    }

    public func exists(_ id: String) -> Bool {
        url(for: id).map { FileManager.default.fileExists(atPath: $0.path) } ?? false
    }

    /// 録音中の一時ファイル(停止して `importFile` するまでの置き場)。
    public static func temporaryRecordingURL(id: String = newID()) -> URL {
        FileManager.default.temporaryDirectory.appendingPathComponent("metamon-rec-\(id).\(fileExtension)")
    }

    /// `source` のファイルを新しい id で取り込む(move。失敗したら copy)。iCloud バックアップの対象外。
    public func importFile(at source: URL, id: String = LocalAudioStore.newID()) throws -> String {
        guard let dest = url(for: id) else { throw CocoaError(.fileWriteInvalidFileName) }
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        try? FileManager.default.removeItem(at: dest)
        do { try FileManager.default.moveItem(at: source, to: dest) } catch { try FileManager.default.copyItem(at: source, to: dest) }
        var u = dest
        var values = URLResourceValues()
        values.isExcludedFromBackup = true
        try? u.setResourceValues(values)
        return id
    }

    public func remove(_ ids: some Sequence<String>) {
        for id in ids { if let u = url(for: id) { try? FileManager.default.removeItem(at: u) } }
    }

    public func removeAll() {
        try? FileManager.default.removeItem(at: directory)
    }
}

/// 録音の上限・表示の決まりごと(UI から切り離して単体テストする)。
public enum AudioRecorderPolicy {
    /// maxDuration の既定(10 分)と上限(2 時間・backend `dsl.AudioMaxDuration` と同じ)。
    public static let defaultMaxDuration = 600
    public static let maxAllowedDuration = 7200

    /// DSL の maxDuration(省略・範囲外も来うる)を 1...7200 秒に丸める。
    public static func maxDuration(_ raw: Int?) -> Int {
        guard let raw, raw > 0 else { return defaultMaxDuration }
        return min(raw, maxAllowedDuration)
    }

    /// 経過時間の表示。1 時間未満は「m:ss」、以上は「h:mm:ss」。
    public static func elapsedText(_ seconds: Double) -> String {
        let t = max(0, Int(seconds.isFinite ? seconds : 0))
        let h = t / 3600, m = (t % 3600) / 60, s = t % 60
        return h > 0 ? String(format: "%d:%02d:%02d", h, m, s) : String(format: "%d:%02d", m, s)
    }

    /// レコードに残す長さ(秒)。端数は切り上げ(0.4 秒の録音も 1 秒)。
    public static func storedSeconds(_ duration: Double) -> Int {
        guard duration.isFinite, duration > 0 else { return 0 }
        return Int(duration.rounded(.up))
    }

    /// 音量メーター: AVAudioRecorder の平均パワー(dBFS・-160...0)を 0...1 に。-50 dB 以下は 0。
    public static let meterFloor: Float = -50
    public static func meterLevel(power: Float) -> Double {
        guard power.isFinite else { return 0 }
        return Double(min(1, max(0, (power - meterFloor) / -meterFloor)))
    }
}

/// 録音部品の状態遷移(#420)。再生は別(`AudioPlayback`)で、ここは録音の有無だけを持つ。
public struct AudioRecorderMachine: Equatable, Sendable {
    public enum Phase: Equatable, Sendable { case idle, recording, recorded }
    public enum Event: Equatable, Sendable {
        case start
        /// 停止ボタン・上限到達・中断(電話など)。いずれも録れた分を残す
        case stop
        /// 録音を始められなかった・ファイルが壊れた。録音前の状態に戻す
        case fail
        case discard
    }

    public private(set) var phase: Phase
    /// 録音を始める前に録音があったか(fail で戻す先)
    private var hadClip: Bool

    public init(hasClip: Bool) {
        phase = hasClip ? .recorded : .idle
        hadClip = hasClip
    }

    /// 受け付けたら true(無効な遷移は状態を変えず false)。
    @discardableResult
    public mutating func send(_ event: Event) -> Bool {
        switch (phase, event) {
        case (.idle, .start), (.recorded, .start):
            hadClip = phase == .recorded
            phase = .recording
        case (.recording, .stop):
            phase = .recorded
        case (.recording, .fail):
            phase = hadClip ? .recorded : .idle
        case (.recorded, .discard):
            phase = .idle
            hadClip = false
        default:
            return false
        }
        return true
    }

    /// イヤホンのボタン(MPRemoteCommandCenter)で何をするか。
    public enum RemoteCommand: Sendable { case togglePlayPause, play, pause }
    public enum RemoteAction: Equatable, Sendable { case startRecording, stopRecording, stopPlayback }

    /// 再生中なら止めるだけ。それ以外は 再生/一時停止 = 録音の開始/停止。
    /// 録音済みでの開始は録り直し(保存するまでは元のレコードは変わらない)。
    public func remoteAction(_ command: RemoteCommand, isPlaying: Bool) -> RemoteAction? {
        if isPlaying { return command == .play ? nil : .stopPlayback }
        switch (command, phase) {
        case (.togglePlayPause, .recording), (.pause, .recording): return .stopRecording
        case (.togglePlayPause, _), (.play, .idle), (.play, .recorded): return .startRecording
        default: return nil
        }
    }
}
