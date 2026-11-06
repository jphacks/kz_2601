import Foundation
import Observation

/// 本命(WhisperKit)の準備ができるまで、仮の聞き取り(Apple の端末内の音声認識)で声を使えるようにする(U5・UX 点検 H2)。
/// - 録音を始める時に選ぶ: 本命が ready なら本命、そうでなく仮が使えるなら仮、どちらも駄目なら本命(従来どおり notReady)。
/// - 録音中に本命の準備が終わっても、その録音は始めた方で止める(切り替えは録音の合間)。
/// - モデルの状態(status・DL 進捗・needsDownload・prepare)は本命のまま見せる(設定・確認アラート・DL の表示は変わらない)。
@MainActor
@Observable
public final class FallbackTranscriber: SpeechTranscriber {
    enum Engine { case primary, provisional }

    @ObservationIgnored public let primary: any SpeechTranscriber
    @ObservationIgnored public let provisional: any ProvisionalTranscriber
    /// 録音中の方(nil = 録音していない)
    private(set) var active: Engine?
    /// 直近に使った方(診断を出す先)
    private var lastUsed: Engine?

    public init(primary: any SpeechTranscriber, provisional: any ProvisionalTranscriber) {
        self.primary = primary
        self.provisional = provisional
    }

    /// 次の録音で使う方
    var nextEngine: Engine {
        if primary.isReady { return .primary }
        return provisional.isAvailable ? .provisional : .primary
    }

    private func engine(_ e: Engine) -> any SpeechTranscriber { e == .primary ? primary : provisional }

    public var isReady: Bool { active != nil || primary.isReady || provisional.isAvailable }
    public var isProvisional: Bool { (active ?? nextEngine) == .provisional }
    public var status: TranscriberStatus { primary.status }
    public var downloadProgress: Double { primary.downloadProgress }
    public var needsDownload: Bool { primary.needsDownload }
    public var lastDiagnostics: TranscriberDiagnostics? { engine(lastUsed ?? .primary).lastDiagnostics }

    public func prepare() async throws { try await primary.prepare() }

    public func startRecording() async throws {
        guard active == nil else { return }
        let e = nextEngine
        lastUsed = e
        try await engine(e).startRecording()
        active = e
    }

    public func stopAndTranscribe() async throws -> String {
        guard let e = active else { return "" }
        defer { active = nil }
        return try await engine(e).stopAndTranscribe()
    }

    public func cancelRecording() {
        guard let e = active else { return }
        engine(e).cancelRecording()
        active = nil
    }
}
