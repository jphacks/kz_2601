import Foundation

/// aiButton の端末内 AI の集計(#446)。Renderer が数え、ホストが 1 日 1 回 `POST /v1/metrics/ai-button` で送る。
/// 値は決まったラベルだけ(利用者の文・記録・DSL の中身は持たない)。ラベルは backend の `aibutton_metrics.go` と同じ。
public enum AIButtonMetric {
    /// Foundation Models の状態。
    public enum Availability: String, Codable, Sendable, CaseIterable {
        case available
        case deviceNotEligible = "device_not_eligible"
        case appleIntelligenceOff = "apple_intelligence_off"
        case modelNotReady = "model_not_ready"
        case osTooOld = "os_too_old"
        case other
    }

    /// DSL の `aiButton.task`。未知の値は other(DSL の文字列をそのまま送らない)。
    public enum Task: String, Codable, Sendable, CaseIterable {
        case summarize, classify, rewrite, extract, suggest, generate, other

        public init(dslTask: String?) {
            self = dslTask.flatMap(Task.init(rawValue:)) ?? .other
        }
    }

    /// 答えを出せなかった理由。
    public enum FailureReason: String, Codable, Sendable, CaseIterable {
        /// 押した時に使えなくなっていた
        case unavailable
        /// 答えを読み取れない・分類が候補外
        case invalidResponse = "invalid_response"
        /// 文脈(約 4K トークン)を超えた
        case contextTooLong = "context_too_long"
        /// 安全の仕組みで止まった
        case guardrail
        /// 言語・地域が非対応
        case unsupportedLanguage = "unsupported_language"
        /// モデルが断った
        case refusal
        /// 端末の上限・同時実行
        case rateLimited = "rate_limited"
        /// モデルのデータが無い
        case assetsUnavailable = "assets_unavailable"
        /// その他のモデルの失敗
        case modelError = "model_error"
        /// DSL の設定が足りない(task・候補・指示が無い)
        case badConfig = "bad_config"
    }

    /// Renderer → ホストに渡す 1 件。
    public enum Event: Equatable, Sendable {
        /// aiButton を表示した時の状態(その日の最後の値を送る)
        case shown(Availability)
        /// 端末内 AI で 1 回実行した。failure が nil なら答えを出せた
        case ran(Task, failure: FailureReason?)
    }
}
