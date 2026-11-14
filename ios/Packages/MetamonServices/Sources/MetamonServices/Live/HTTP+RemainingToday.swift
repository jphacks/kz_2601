import Foundation

/// 応答の `X-RateLimit-*` を「今日の残り」に渡す(#630 M1)。ヘッダーを付けるのは ai だけなので、他の応答では何もしない。
extension HTTP {
    /// テストで差し替える。既定は `RemainingTodayStore.shared` に入れる
    nonisolated(unsafe) static var remainingSink: @Sendable (RemainingToday) -> Void = { value in
        Task { @MainActor in RemainingTodayStore.shared.record(value) }
    }

    static func noteRemaining(_ r: Response, now: Date = .now) {
        guard let value = RemainingToday.parse(header: r.header, status: r.status, body: r.data, now: now) else { return }
        remainingSink(value)
    }

    /// ストリーム(SSE・#636 の `openStream`)の応答用。本文はまだ無いので、ヘッダーだけで読む
    static func noteRemaining(_ http: HTTPURLResponse?, now: Date = .now) {
        guard let http else { return }
        noteRemaining(Response(status: http.statusCode, data: Data(), headers: http.allHeaderFields), now: now)
    }
}
