import Foundation

/// 固定文言を返す文字起こしモック(シミュレータ / テスト用)。
@MainActor
public final class MockTranscriber: SpeechTranscriber {
    public var isReady: Bool { true }
    public var cannedText: String
    public var delay: Duration

    public init(cannedText: String = "今月の支出を記録できる家計簿アプリが欲しい", delay: Duration = .milliseconds(800)) {
        self.cannedText = cannedText
        self.delay = delay
    }

    public func prepare() async throws {}
    public func startRecording() async throws {}

    public func stopAndTranscribe() async throws -> String {
        try await Task.sleep(for: delay)
        return cannedText
    }
}
