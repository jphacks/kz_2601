import Foundation

/// `musicPlayer`(#412)の 1 曲。Apple Music のカタログ id があれば id で、無ければ曲名 + アーティストで探す。
/// 近くの端末から届くので、使う前に必ず `MusicRequestRules.sanitize` を通す。
public struct MusicSong: Codable, Hashable, Sendable {
    public var id: String?
    public var title: String
    public var artist: String
    public var artworkURL: String?

    public init(id: String? = nil, title: String, artist: String, artworkURL: String? = nil) {
        self.id = id
        self.title = title
        self.artist = artist
        self.artworkURL = artworkURL
    }

    /// 同じ曲か(両方に id があれば id、無ければ曲名 + アーティストを大文字小文字・前後の空白を無視して比べる)。
    public func isSameSong(as other: MusicSong) -> Bool {
        if let a = id, let b = other.id { return a == b }
        return Self.norm(title) == Self.norm(other.title) && Self.norm(artist) == Self.norm(other.artist)
    }

    private static func norm(_ s: String) -> String {
        s.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
    }
}

/// 届いたリクエストの上限と検証(壊れた・悪意ある相手から大きな値を入れられないように)。
public enum MusicRequestRules {
    public static let maxTitle = 100
    public static let maxArtist = 100
    public static let maxID = 40
    public static let maxArtworkURL = 512
    public static let maxSender = 63
    /// 承認待ちの上限。超えたら古い順に捨てる。
    public static let maxPending = 20

    /// 前後の空白を落として長さを切る。曲名が空なら nil。id は英数字とドットだけ、ジャケットは https だけ残す。
    public static func sanitize(_ song: MusicSong) -> MusicSong? {
        let title = clip(song.title, maxTitle)
        guard !title.isEmpty else { return nil }
        let id = song.id.map { $0.trimmingCharacters(in: .whitespaces) }.flatMap { s -> String? in
            guard !s.isEmpty, s.count <= maxID, s.allSatisfy({ $0.isASCII && ($0.isLetter || $0.isNumber || $0 == ".") }) else { return nil }
            return s
        }
        let artwork = song.artworkURL.flatMap { s -> String? in
            guard s.count <= maxArtworkURL, let url = URL(string: s), url.scheme == "https", url.host != nil else { return nil }
            return s
        }
        return MusicSong(id: id, title: title, artist: clip(song.artist, maxArtist), artworkURL: artwork)
    }

    public static func clip(_ s: String, _ max: Int) -> String {
        String(s.trimmingCharacters(in: .whitespacesAndNewlines).prefix(max))
    }
}

/// 友だちから届いた 1 件(承認待ち)。
public struct MusicRequest: Identifiable, Equatable, Sendable {
    public let id: UUID
    public var song: MusicSong
    public var sender: String

    public init(id: UUID = UUID(), song: MusicSong, sender: String) {
        self.id = id
        self.song = song
        self.sender = sender
    }
}

/// 届いたリクエストの受け皿。再生(MusicKit)とは切り離した純粋ロジック。
public struct MusicRequestInbox: Equatable, Sendable {
    public private(set) var pending: [MusicRequest] = []

    public enum Outcome: Equatable, Sendable {
        /// すぐ再生待ちに入れる(autoAccept)
        case queue(MusicSong)
        /// 承認待ちに積んだ
        case pending
        /// 承認待ち・再生待ちに同じ曲がある
        case duplicate
        /// 曲名が無い等で受け取れない
        case invalid
    }

    public init() {}

    /// `queued` は今の再生待ち(重複の判定に使う)。
    public mutating func receive(_ song: MusicSong, from sender: String, autoAccept: Bool, queued: [MusicSong] = []) -> Outcome {
        guard let song = MusicRequestRules.sanitize(song) else { return .invalid }
        if pending.contains(where: { $0.song.isSameSong(as: song) }) || queued.contains(where: { $0.isSameSong(as: song) }) {
            return .duplicate
        }
        if autoAccept { return .queue(song) }
        var name = MusicRequestRules.clip(sender, MusicRequestRules.maxSender)
        if name.isEmpty { name = "近くの人" }
        pending.append(MusicRequest(song: song, sender: name))
        if pending.count > MusicRequestRules.maxPending { pending.removeFirst(pending.count - MusicRequestRules.maxPending) }
        return .pending
    }

    /// 承認: 承認待ちから外して返す(呼び出し側が再生待ちに入れる)。
    public mutating func accept(_ id: UUID) -> MusicRequest? {
        guard let i = pending.firstIndex(where: { $0.id == id }) else { return nil }
        return pending.remove(at: i)
    }

    public mutating func reject(_ id: UUID) {
        pending.removeAll { $0.id == id }
    }

    /// 承認に失敗した時に戻す(先頭へ)。
    public mutating func restore(_ request: MusicRequest) {
        guard !pending.contains(where: { $0.id == request.id }) else { return }
        pending.insert(request, at: 0)
    }
}

/// 外部アプリへのリンク。Spotify は SDK を使わず(登録・Premium・審査が要る)、検索ページを開くだけ。
public enum MusicLinks {
    /// Apple Music / Spotify の検索語(曲名 + アーティスト)。
    public static func searchTerm(_ song: MusicSong) -> String {
        [song.title, song.artist].map { $0.trimmingCharacters(in: .whitespaces) }.filter { !$0.isEmpty }.joined(separator: " ")
    }

    /// `https://open.spotify.com/search/<曲名 アーティスト>`(Spotify アプリがあれば OS がアプリで開く)。
    public static func spotifySearchURL(_ song: MusicSong) -> URL? {
        let term = searchTerm(song)
        guard !term.isEmpty else { return nil }
        var allowed = CharacterSet.urlPathAllowed
        allowed.remove(charactersIn: "/?#")
        guard let path = term.addingPercentEncoding(withAllowedCharacters: allowed) else { return nil }
        return URL(string: "https://open.spotify.com/search/" + path)
    }
}

public extension AppDSL {
    /// 最初の `musicPlayer`(container の子も見る)。端末のプレーヤーは 1 つなので 1 アプリ 1 つ。
    var musicPlayer: Component? {
        for s in screens {
            for c in s.components {
                if c.type == .musicPlayer { return c }
                if let child = c.children.first(where: { $0.type == .musicPlayer }) { return child }
            }
        }
        return nil
    }

    /// 近くの端末から曲のリクエストを受けるか。
    var usesMusicRequests: Bool { musicPlayer?.allowRequests == true }
}
