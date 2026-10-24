import Foundation
import MetamonDSL

/// `POST /v1/generate` の結果。`document` は適用済みの最新 DSL 封筒、`summary` は Claude が付けた変更要約(UI に表示)。
public struct MorphResult: Sendable {
    public var document: AppDocument
    public var summary: String?
    /// full | patch(サーバーがどう変えたか。診断用)
    public var mode: String?

    public init(document: AppDocument, summary: String? = nil, mode: String? = nil) {
        self.document = document
        self.summary = summary
        self.mode = mode
    }
}

/// ai サービス: 発話テキスト → (Claude が DSL / JSON Patch 生成 → appconfig に適用) → 最新 DSL 封筒 + 要約を返す。
public protocol AIClient: Sendable {
    func morph(transcript: String, appId: String, currentVersion: Int) async throws -> MorphResult
}
