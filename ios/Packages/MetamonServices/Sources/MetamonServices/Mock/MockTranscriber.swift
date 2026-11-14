import Foundation

/// 固定文言を返す文字起こしモック(シミュレータ / テスト用)。
@MainActor
public final class MockTranscriber: SpeechTranscriber, AudioFileTranscriber {
    public var isReady: Bool { true }
    public var cannedText: String
    public var delay: Duration

    public init(cannedText: String = "今月の支出を記録できる家計簿アプリが欲しい", delay: Duration = .milliseconds(800)) {
        self.cannedText = cannedText
        self.delay = delay
    }

    public func prepare() async throws {}
    /// 録音中か(テストで「画面を閉じたら録音が止まる」を確かめる)
    public private(set) var isRecording = false

    public func startRecording() async throws { isRecording = true }

    public func stopAndTranscribe() async throws -> String {
        isRecording = false
        try await Task.sleep(for: delay)
        return cannedText
    }

    public func cancelRecording() { isRecording = false }

    public var canTranscribeFiles: Bool { !isRecording }

    /// ファイルは読まず、4 段で進んで固定文言を返す(シミュレータで進み具合・キャンセルを見る)。
    public func transcribeFile(at url: URL, progress: @escaping @MainActor (Double) -> Void) async throws -> String {
        for step in 1...4 {
            try await Task.sleep(for: delay)
            progress(Double(step) / 4)
        }
        return "(録音ファイル \(url.lastPathComponent) の文字起こしの例)" + cannedText
    }
}
