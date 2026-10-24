import Foundation
import MetamonDSL

/// 発話のキーワードで同梱サンプル DSL(docs/dsl/examples)を返す ai モック。
public struct MockAIClient: AIClient {
    public var delay: Duration

    public init(delay: Duration = .seconds(1.2)) { self.delay = delay }

    public static func sampleName(for transcript: String) -> String {
        if transcript.contains("トレ") || transcript.contains("筋") || transcript.contains("運動") { return "training" }
        if transcript.contains("散歩") || transcript.contains("写真") || transcript.contains("場所") { return "walk" }
        if transcript.contains("最初") || transcript.contains("戻") { return "initial" }
        return "kakeibo"
    }

    public func morph(transcript: String, appId: String, currentVersion: Int) async throws -> AppDocument {
        try await Task.sleep(for: delay)
        return try DSLExamples.document(Self.sampleName(for: transcript), version: currentVersion + 1)
    }
}
