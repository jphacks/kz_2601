import Foundation
import MusicKit
import MetamonDSL

/// Apple Music の「最近聞いた曲」を読むだけの口(#1988)。部品・スキーマはまだ無い(#1698 の決定待ち)。
/// 端末の中だけで使い、サーバーには送らない。テスト・`METAMON_MOCK=1` は `FakeRecentlyPlayed`。
@MainActor
public protocol RecentlyPlayedSource: AnyObject {
    /// 使えるか。`requesting` なら未確認の許可をこの場で聞く。
    func availability(requesting: Bool) async -> MusicAvailability
    /// 新しい順。`limit` は 1...`RecentlyPlayed.maxLimit` に丸める(0 以下は空)。読めない状態なら `MusicPlaybackError.unavailable`。
    func recentlyPlayed(limit: Int) async throws -> [MusicSong]
}

public enum RecentlyPlayed {
    /// 一度に読む上限
    public static let maxLimit = 25

    /// アプリ起動時に差し替えられる(`METAMON_MOCK=1` は `FakeRecentlyPlayed`)。
    @MainActor public static var source: any RecentlyPlayedSource = AppleRecentlyPlayed()

    static func clamp(_ limit: Int) -> Int { min(max(limit, 0), maxLimit) }
}

extension MusicAvailability {
    /// 最近聞いた曲を読めるか。許可があれば読む(契約の要否は実機で確かめる・#1988)。
    public var canReadRecentlyPlayed: Bool { self == .ready || self == .noSubscription }
}

/// 実体: MusicKit の `MusicRecentlyPlayedRequest<Song>`(iOS 16 / macOS 13 から。パッケージの最低版より古いので `@available` は不要)。
/// Mac で同じ実体が動くかはまだ確かめていない。
@MainActor
public final class AppleRecentlyPlayed: RecentlyPlayedSource {
    private let authorization: @MainActor (Bool) async -> MusicAvailability

    /// 履歴を読むのに要るのは許可だけ。契約(`MusicSubscription.current`)は読み込まない。テストは `authorization` を差し替える。
    public init(authorization: (@MainActor (Bool) async -> MusicAvailability)? = nil) {
        self.authorization = authorization ?? { requesting in
            var status = MusicAuthorization.currentStatus
            if status == .notDetermined && requesting { status = await MusicAuthorization.request() }
            return Self.availability(for: status)
        }
    }

    public func availability(requesting: Bool) async -> MusicAvailability {
        await authorization(requesting)
    }

    public func recentlyPlayed(limit: Int) async throws -> [MusicSong] {
        let n = RecentlyPlayed.clamp(limit)
        guard n > 0 else { return [] }
        let state = await availability(requesting: false)
        guard state.canReadRecentlyPlayed else { throw MusicPlaybackError.unavailable(state) }
        var req = MusicRecentlyPlayedRequest<Song>()
        req.limit = n
        let items: MusicItemCollection<Song>
        do {
            items = try await req.response().items
        } catch is CancellationError {
            throw CancellationError()
        } catch {
            // MusicKit の生のエラーは部品に出さない。契約が無いか、それ以外は「使えない」。
            let sub = try? await MusicSubscription.current
            throw MusicPlaybackError.unavailable(Self.failureState(canPlayCatalog: sub?.canPlayCatalogContent))
        }
        return items.prefix(n).map {
            MusicSong(id: $0.id.rawValue, title: $0.title, artist: $0.artistName,
                      artworkURL: Self.httpsArtwork($0.artwork?.url(width: 200, height: 200)))
        }
    }

    /// 許可の状態だけで決める(`.authorized` なら読める)。
    nonisolated static func availability(for status: MusicAuthorization.Status) -> MusicAvailability {
        switch status {
        case .authorized: .ready
        case .notDetermined: .notDetermined
        case .denied: .denied
        case .restricted: .restricted
        @unknown default: .unavailable
        }
    }

    /// 読み込みに失敗した時の状態。契約が無いと分かれば `.noSubscription`、それ以外は `.unavailable`。
    nonisolated static func failureState(canPlayCatalog: Bool?) -> MusicAvailability {
        canPlayCatalog == false ? .noSubscription : .unavailable
    }

    /// `https` の画像だけ使う(`musicKit://` などは画像部品で読めない)。
    nonisolated static func httpsArtwork(_ url: URL?) -> String? {
        guard let url, url.scheme?.lowercased() == "https" else { return nil }
        return url.absoluteString
    }
}

/// `METAMON_MOCK=1` / テスト / プレビュー用。固定の履歴(新しい順)。
@MainActor
public final class FakeRecentlyPlayed: RecentlyPlayedSource {
    public static let history: [MusicSong] = FakeMusicPlayback.catalog.reversed()

    public var state: MusicAvailability
    public var items: [MusicSong]
    /// 最後に読んだ時の件数(上限の確認用)
    public private(set) var lastLimit: Int?

    public init(state: MusicAvailability = .ready, items: [MusicSong]? = nil) {
        self.state = state
        self.items = items ?? Self.history
    }

    public func availability(requesting: Bool) async -> MusicAvailability {
        if requesting && state == .notDetermined { state = .ready }
        return state
    }

    public func recentlyPlayed(limit: Int) async throws -> [MusicSong] {
        let n = RecentlyPlayed.clamp(limit)
        lastLimit = n
        guard n > 0 else { return [] }
        guard state.canReadRecentlyPlayed else { throw MusicPlaybackError.unavailable(state) }
        return Array(items.prefix(n))
    }
}
