#if os(iOS)
import Foundation
import MusicKit
import MetamonDSL

/// `musicPlayer`(#412)の実体: MusicKit の `ApplicationMusicPlayer`(アプリ専用の再生待ち・システムの再生 UI に出る)。
/// - 許可: `NSAppleMusicUsageDescription`。カタログの再生は Apple Music の契約が要る(`MusicSubscription`)
/// - App ID に MusicKit(App Services)が要る。無いとカタログの取得が失敗する(docs/ios/architecture.md)
/// - イヤホンのボタン・ロック画面・コントロールセンターは OS が `ApplicationMusicPlayer` を直接操作する
@MainActor
public final class AppleMusicPlayback: MusicPlayback {
    private var player: ApplicationMusicPlayer { .shared }

    public init() {}

    public func availability(requesting: Bool) async -> MusicAvailability {
        var status = MusicAuthorization.currentStatus
        if status == .notDetermined && requesting { status = await MusicAuthorization.request() }
        switch status {
        case .notDetermined: return .notDetermined
        case .denied: return .denied
        case .restricted: return .restricted
        case .authorized: break
        @unknown default: return .unavailable
        }
        guard let sub = try? await MusicSubscription.current else { return .unavailable }
        return sub.canPlayCatalogContent ? .ready : .noSubscription
    }

    public func search(_ term: String) async throws -> [MusicSong] {
        let t = term.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !t.isEmpty else { return [] }
        var req = MusicCatalogSearchRequest(term: String(t.prefix(100)), types: [Song.self])
        req.limit = 15
        return try await req.response().songs.map(Self.song)
    }

    public func enqueue(_ song: MusicSong) async throws -> MusicSong {
        let item = try await resolve(song)
        if player.queue.entries.isEmpty {
            player.queue = ApplicationMusicPlayer.Queue(for: [item])
        } else {
            try await player.queue.insert(item, position: .tail)
        }
        return Self.song(item)
    }

    public func play() async throws {
        if !player.isPreparedToPlay { try await player.prepareToPlay() }
        try await player.play()
    }

    public func pause() { player.pause() }

    public func skipToNext() async throws { try await player.skipToNextEntry() }

    public func skipToPrevious() async throws { try await player.skipToPreviousEntry() }

    public func snapshot() -> MusicPlaybackSnapshot {
        let entries = Array(player.queue.entries)
        let current = player.queue.currentEntry
        let start = current.flatMap { c in entries.firstIndex { $0.id == c.id } }.map { $0 + 1 } ?? 0
        return MusicPlaybackSnapshot(
            isPlaying: player.state.playbackStatus == .playing,
            nowPlaying: current.map(Self.song),
            upNext: entries.dropFirst(start).map(Self.song)
        )
    }

    /// id があればカタログから、無ければ「曲名 アーティスト」で検索した最初の 1 曲。
    private func resolve(_ song: MusicSong) async throws -> Song {
        if let id = song.id {
            let req = MusicCatalogResourceRequest<Song>(matching: \.id, equalTo: MusicItemID(id))
            if let found = try await req.response().items.first { return found }
        }
        let term = MusicLinks.searchTerm(song)
        var req = MusicCatalogSearchRequest(term: term, types: [Song.self])
        req.limit = 1
        guard let found = try await req.response().songs.first else { throw MusicPlaybackError.notFound(term) }
        return found
    }

    private static func song(_ s: Song) -> MusicSong {
        MusicSong(id: s.id.rawValue, title: s.title, artist: s.artistName,
                  artworkURL: s.artwork?.url(width: 200, height: 200)?.absoluteString)
    }

    private static func song(_ e: MusicPlayer.Queue.Entry) -> MusicSong {
        var id: String?
        if case .song(let s) = e.item { id = s.id.rawValue }
        return MusicSong(id: id, title: e.title, artist: e.subtitle ?? "",
                         artworkURL: e.artwork?.url(width: 200, height: 200)?.absoluteString)
    }
}
#endif
