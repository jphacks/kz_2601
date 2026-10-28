import Foundation
import MetamonDSL

/// `musicPlayer`(#412)の再生元。実体は MusicKit(`AppleMusicPlayback`)。テスト・`METAMON_MOCK=1` は `FakeMusicPlayback`。
/// システムのプレーヤーを使うので、イヤホンのボタン・ロック画面・コントロールセンターの操作は OS がそのまま扱う。
@MainActor
public protocol MusicPlayback: AnyObject {
    /// 使えるか。`requesting` なら未確認の許可をこの場で聞く。
    func availability(requesting: Bool) async -> MusicAvailability
    func search(_ term: String) async throws -> [MusicSong]
    /// 再生待ちの最後に入れる。id が無ければ曲名 + アーティストで探した最初の 1 曲。入れた曲を返す。
    func enqueue(_ song: MusicSong) async throws -> MusicSong
    func play() async throws
    func pause()
    func skipToNext() async throws
    func skipToPrevious() async throws
    func snapshot() -> MusicPlaybackSnapshot
}

/// Apple Music を使えるか。メッセージは部品にそのまま出す。
public enum MusicAvailability: Equatable, Sendable {
    /// まだ許可を聞いていない
    case notDetermined
    case ready
    case denied
    case restricted
    /// 許可はあるが Apple Music の契約が無い(検索・リクエストはできる)
    case noSubscription
    case unavailable

    public var canSearch: Bool { self == .ready || self == .noSubscription }
    public var canPlay: Bool { self == .ready }

    public var message: String? {
        switch self {
        case .notDetermined, .ready: nil
        case .denied: L("Apple Music へのアクセスが許可されていません。設定アプリの Tailor It で「メディアと Apple Music」をオンにしてください")
        case .restricted: L("この端末では Apple Music の利用が制限されています")
        case .noSubscription: L("曲を再生するには Apple Music の契約が必要です")
        case .unavailable: L("この端末では Apple Music を使えません")
        }
    }
}

public struct MusicPlaybackSnapshot: Equatable, Sendable {
    public var isPlaying = false
    public var nowPlaying: MusicSong?
    public var upNext: [MusicSong] = []

    public init(isPlaying: Bool = false, nowPlaying: MusicSong? = nil, upNext: [MusicSong] = []) {
        self.isPlaying = isPlaying
        self.nowPlaying = nowPlaying
        self.upNext = upNext
    }

    /// 今の曲 + 再生待ち(重複の判定用)。
    public var all: [MusicSong] { (nowPlaying.map { [$0] } ?? []) + upNext }
}

public enum MusicPlaybackError: LocalizedError, Equatable {
    case notFound(String)
    case unavailable(MusicAvailability)

    public var errorDescription: String? {
        switch self {
        case .notFound(let term): L("Apple Music で「\(term)」が見つかりませんでした")
        case .unavailable(let a): a.message ?? L("Apple Music を使えません")
        }
    }
}

/// `METAMON_MOCK=1` / テスト / プレビュー用。固定のカタログ・メモリ上の再生待ち(音は出さない)。
@MainActor
public final class FakeMusicPlayback: MusicPlayback {
    public static let catalog: [MusicSong] = [
        MusicSong(id: "1001", title: "夜明けのドライブ", artist: "テイラーズ"),
        MusicSong(id: "1002", title: "海沿いの道", artist: "テイラーズ"),
        MusicSong(id: "1003", title: "Highway Sunset", artist: "The Samples"),
        MusicSong(id: "1004", title: "サービスエリア", artist: "みどりの窓"),
        MusicSong(id: "1005", title: "Night Radio", artist: "The Samples"),
    ]

    public var state: MusicAvailability
    /// true = カタログに無い曲は入れられない(見つからない時の確認用)
    public var strict: Bool
    public private(set) var history: [MusicSong] = []
    public private(set) var current: MusicSong?
    public private(set) var queue: [MusicSong] = []
    public private(set) var isPlaying = false

    public init(state: MusicAvailability = .ready, strict: Bool = false) {
        self.state = state
        self.strict = strict
    }

    public func availability(requesting: Bool) async -> MusicAvailability {
        if requesting && state == .notDetermined { state = .ready }
        return state
    }

    public func search(_ term: String) async throws -> [MusicSong] {
        guard state.canSearch else { throw MusicPlaybackError.unavailable(state) }
        let t = term.trimmingCharacters(in: .whitespaces).lowercased()
        guard !t.isEmpty else { return Self.catalog }
        return Self.catalog.filter { s in
            t.split(separator: " ").allSatisfy { w in (s.title + " " + s.artist).lowercased().contains(w) }
        }
    }

    public func enqueue(_ song: MusicSong) async throws -> MusicSong {
        guard state.canPlay else { throw MusicPlaybackError.unavailable(state) }
        var found = Self.catalog.first { $0.isSameSong(as: song) }
        if found == nil && song.id == nil { found = try await search(MusicLinks.searchTerm(song)).first }
        guard let item = found ?? (strict ? nil : song) else { throw MusicPlaybackError.notFound(MusicLinks.searchTerm(song)) }
        if current == nil { current = item } else { queue.append(item) }
        return item
    }

    public func play() async throws {
        guard state.canPlay else { throw MusicPlaybackError.unavailable(state) }
        isPlaying = current != nil
    }

    public func pause() { isPlaying = false }

    public func skipToNext() async throws {
        guard !queue.isEmpty else { return }
        if let c = current { history.append(c) }
        current = queue.removeFirst()
    }

    public func skipToPrevious() async throws {
        guard let prev = history.popLast() else { return }
        if let c = current { queue.insert(c, at: 0) }
        current = prev
    }

    public func snapshot() -> MusicPlaybackSnapshot {
        MusicPlaybackSnapshot(isPlaying: isPlaying, nowPlaying: current, upNext: queue)
    }
}
