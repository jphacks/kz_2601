import XCTest
@testable import MetamonDSL

/// `musicPlayer`(#412): 曲のリクエストの検証・承認待ち・近距離共有の判定・Spotify のリンク。
final class MusicRequestTests: XCTestCase {
    private let song = MusicSong(id: "1440857781", title: "夜に駆ける", artist: "YOASOBI", artworkURL: "https://example.com/a.jpg")

    func testExampleDecodes() throws {
        let dsl = try ExamplesFixture.load("drive_music")
        let player = try XCTUnwrap(dsl.musicPlayer)
        XCTAssertEqual(player.type, .musicPlayer)
        XCTAssertEqual(player.allowRequests, true)
        XCTAssertNil(player.autoAccept)
        XCTAssertTrue(dsl.usesMusicRequests)
    }

    func testSanitizeClipsAndDropsBadValues() throws {
        let long = String(repeating: "あ", count: 300)
        let s = try XCTUnwrap(MusicRequestRules.sanitize(MusicSong(id: "../etc", title: "  \(long) ", artist: long, artworkURL: "http://x/a.jpg")))
        XCTAssertEqual(s.title.count, MusicRequestRules.maxTitle)
        XCTAssertEqual(s.artist.count, MusicRequestRules.maxArtist)
        XCTAssertNil(s.id, "英数字とドット以外の id は捨てる")
        XCTAssertNil(s.artworkURL, "https 以外のジャケットは捨てる")
        XCTAssertNil(MusicRequestRules.sanitize(MusicSong(title: "   ", artist: "x")), "曲名が空なら受け取らない")
        XCTAssertEqual(MusicRequestRules.sanitize(song), song)
        XCTAssertNil(MusicRequestRules.sanitize(MusicSong(id: String(repeating: "1", count: 41), title: "a", artist: ""))?.id)
    }

    func testInboxPendingAcceptReject() throws {
        var inbox = MusicRequestInbox()
        XCTAssertEqual(inbox.receive(song, from: "花子", autoAccept: false), .pending)
        XCTAssertEqual(inbox.receive(MusicSong(id: "1440857781", title: "別名", artist: ""), from: "太郎", autoAccept: false), .duplicate, "同じ id")
        XCTAssertEqual(inbox.receive(MusicSong(title: "Lemon", artist: "米津玄師"), from: "", autoAccept: false), .pending)
        XCTAssertEqual(inbox.receive(MusicSong(title: " lemon ", artist: "米津玄師"), from: "x", autoAccept: false), .duplicate, "曲名 + アーティスト")
        XCTAssertEqual(inbox.pending.map(\.sender), ["花子", "近くの人"])

        let first = try XCTUnwrap(inbox.pending.first)
        XCTAssertEqual(inbox.accept(first.id)?.song, song)
        XCTAssertNil(inbox.accept(first.id), "2 回目は無い")
        inbox.reject(inbox.pending[0].id)
        XCTAssertTrue(inbox.pending.isEmpty)
        inbox.restore(first)
        XCTAssertEqual(inbox.pending, [first])
    }

    func testInboxAutoAcceptAndQueueDuplicate() {
        var inbox = MusicRequestInbox()
        XCTAssertEqual(inbox.receive(song, from: "花子", autoAccept: true), .queue(song))
        XCTAssertTrue(inbox.pending.isEmpty)
        XCTAssertEqual(inbox.receive(song, from: "花子", autoAccept: true, queued: [song]), .duplicate, "再生待ちにある曲")
        XCTAssertEqual(inbox.receive(MusicSong(title: "", artist: "x"), from: "a", autoAccept: true), .invalid)
    }

    func testInboxKeepsNewestWithinLimit() {
        var inbox = MusicRequestInbox()
        for i in 0..<(MusicRequestRules.maxPending + 5) {
            _ = inbox.receive(MusicSong(title: "曲\(i)", artist: ""), from: "a", autoAccept: false)
        }
        XCTAssertEqual(inbox.pending.count, MusicRequestRules.maxPending)
        XCTAssertEqual(inbox.pending.first?.song.title, "曲5", "古い順に捨てる")
    }

    func testSongRequestOverNearby() throws {
        let dsl = try ExamplesFixture.load("drive_music")
        let payload = try XCTUnwrap(NearbyShare.makeSongRequest(song, sender: "花子", appId: "other", appName: "ドライブ BGM"))
        XCTAssertEqual(payload.header.count, 1)
        XCTAssertTrue(payload.records.isEmpty)
        let header = try NearbyShare.decode(NearbyShare.Header.self, from: NearbyShare.encode(payload.header))
        XCTAssertEqual(header.song, song)
        XCTAssertEqual(NearbyShare.decide(header, dsl: dsl, appId: "mine"), .accept(entityId: NearbyShare.songEntityId, sameApp: false))
        XCTAssertNil(NearbyShare.makeSongRequest(MusicSong(title: " ", artist: ""), sender: "a", appId: "a", appName: "a"))

        // リクエストを受けないアプリでは断る
        let kakeibo = try ExamplesFixture.load("kakeibo")
        guard case .reject = NearbyShare.decide(header, dsl: kakeibo, appId: "mine") else { return XCTFail("reject 期待") }
    }

    func testRecordHeaderEncodingHasNoSongKey() throws {
        let dsl = try ExamplesFixture.load("attendance_nearby")
        let entity = try XCTUnwrap(dsl.entity("attendance"))
        let data = try NearbyShare.encode(NearbyShare.Header(sender: "s", appId: "a", appName: "x", entity: entity, count: 1))
        XCTAssertFalse(String(decoding: data, as: UTF8.self).contains("song"), "レコードの共有は今までと同じ形")
    }

    func testSpotifySearchURL() throws {
        let url = try XCTUnwrap(MusicLinks.spotifySearchURL(MusicSong(title: "夜に駆ける", artist: "YOASOBI / ayase")))
        XCTAssertEqual(url.host, "open.spotify.com")
        XCTAssertTrue(url.absoluteString.hasPrefix("https://open.spotify.com/search/"))
        let encoded = url.absoluteString.dropFirst("https://open.spotify.com/search/".count)
        XCTAssertFalse(encoded.contains("/"), "スラッシュはエンコードする")
        XCTAssertEqual(encoded.removingPercentEncoding, "夜に駆ける YOASOBI / ayase")
        XCTAssertNil(MusicLinks.spotifySearchURL(MusicSong(title: " ", artist: "")))
    }
}
