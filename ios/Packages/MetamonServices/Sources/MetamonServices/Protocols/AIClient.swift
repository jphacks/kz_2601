import Foundation
import MetamonDSL

/// ai サービス: 発話テキスト → (Claude が DSL / JSON Patch 生成 → appconfig に適用) → 最新 DSL 封筒を返す。
public protocol AIClient: Sendable {
    func morph(transcript: String, appId: String, currentVersion: Int) async throws -> AppDocument
}
