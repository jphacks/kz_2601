import Foundation
import Observation
import MetamonDSL

/// `musicPlayer`(#412)の状態: 再生元(`MusicPlayback`)・今の曲と再生待ち・友だちから届いたリクエスト。
/// 端末のプレーヤーは 1 つなので共有の 1 個(`shared`)。部品とアプリ本体(近くの端末からの受信)が同じものを使う。
@MainActor
@Observable
public final class MusicSession {
    public static let shared = MusicSession()

    /// アプリ起動時に差し替えられる(`METAMON_MOCK=1` は `FakeMusicPlayback`)。
    @ObservationIgnored public var player: any MusicPlayback
    public private(set) var availability: MusicAvailability = .notDetermined
    public private(set) var state = MusicPlaybackSnapshot()
    public private(set) var inbox = MusicRequestInbox()
    /// 部品に出す直近のお知らせ(追加した・届いた・失敗した)
    public var notice: String?

    public init(player: (any MusicPlayback)? = nil) {
        #if os(iOS)
        self.player = player ?? AppleMusicPlayback()
        #else
        self.player = player ?? FakeMusicPlayback()
        #endif
    }

    // MARK: - 再生

    public func prepare(requesting: Bool) async {
        availability = await player.availability(requesting: requesting)
        refresh()
    }

    /// プレーヤーの今の状態を読み直す(ロック画面・イヤホンで操作された分も拾う)。
    public func refresh() {
        let s = player.snapshot()
        if s != state { state = s }
    }

    public func togglePlay() async {
        if state.isPlaying {
            player.pause()
        } else {
            await perform { try await self.player.play() }
        }
        refresh()
    }

    public func skipToNext() async {
        await perform { try await self.player.skipToNext() }
    }

    public func skipToPrevious() async {
        await perform { try await self.player.skipToPrevious() }
    }

    public func search(_ term: String) async -> Result<[MusicSong], MusicPlaybackError> {
        do {
            return .success(try await player.search(term))
        } catch let e as MusicPlaybackError {
            return .failure(e)
        } catch {
            return .failure(.notFound(term))
        }
    }

    /// 再生待ちに入れる。何も流れていなければそのまま再生を始める。入れられたら true。
    @discardableResult
    public func add(_ song: MusicSong) async -> Bool {
        let startPlaying = player.snapshot().nowPlaying == nil
        do {
            let added = try await player.enqueue(song)
            if startPlaying { try? await player.play() }
            notice = L("「\(added.title)」を再生待ちに入れました")
            refresh()
            return true
        } catch {
            notice = (error as? LocalizedError)?.errorDescription ?? L("曲を入れられませんでした")
            refresh()
            return false
        }
    }

    // MARK: - 友だちからのリクエスト

    /// 近くの端末から届いた 1 曲。`autoAccept` なら確認せずに再生待ちへ。戻り値は送った側に返す結果。
    public func receive(_ song: MusicSong, from sender: String, autoAccept: Bool) async -> NearbyShare.Ack {
        switch inbox.receive(song, from: sender, autoAccept: autoAccept, queued: state.all) {
        case .queue(let s):
            guard await add(s) else { return .init(ok: false, added: 0, message: L("相手の端末で曲を再生待ちに入れられませんでした")) }
            notice = L("\(sender)さんのリクエスト「\(s.title)」を再生待ちに入れました")
            return .init(ok: true, added: 1, message: nil)
        case .pending:
            notice = L("\(sender)さんから「\(MusicRequestRules.clip(song.title, MusicRequestRules.maxTitle))」のリクエストが届きました")
            return .init(ok: true, added: 1, message: nil)
        case .duplicate:
            return .init(ok: false, added: 0, message: L("同じ曲がもう入っています"))
        case .invalid:
            return .init(ok: false, added: 0, message: L("曲名のないリクエストは受け取れません"))
        }
    }

    /// 承認: 再生待ちに入れる。入れられなければ承認待ちに戻す。
    public func accept(_ id: UUID) async {
        guard let request = inbox.accept(id) else { return }
        if !(await add(request.song)) { inbox.restore(request) }
    }

    public func reject(_ id: UUID) {
        inbox.reject(id)
    }

    private func perform(_ op: @escaping () async throws -> Void) async {
        do {
            try await op()
        } catch {
            notice = (error as? LocalizedError)?.errorDescription ?? L("操作できませんでした")
        }
        refresh()
    }
}
