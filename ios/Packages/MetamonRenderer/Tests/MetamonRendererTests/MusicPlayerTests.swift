import XCTest
import SwiftUI
import MetamonDSL
@testable import MetamonRenderer

/// `musicPlayer`(#412): 再生待ち・リクエストの受け取り(偽のプレーヤー)と描画。
@MainActor
final class MusicPlayerTests: XCTestCase {
    private func session(_ state: MusicAvailability = .ready, strict: Bool = false) -> (MusicSession, FakeMusicPlayback) {
        let fake = FakeMusicPlayback(state: state, strict: strict)
        return (MusicSession(player: fake), fake)
    }

    func testAddStartsPlayingAndQueuesTheRest() async {
        let (s, fake) = session()
        await s.prepare(requesting: false)
        let ok1 = await s.add(FakeMusicPlayback.catalog[0])
        let ok2 = await s.add(FakeMusicPlayback.catalog[1])
        XCTAssertTrue(ok1 && ok2)
        XCTAssertEqual(s.state.nowPlaying?.id, "1001")
        XCTAssertTrue(s.state.isPlaying, "空の再生待ちに入れたら再生を始める")
        XCTAssertEqual(s.state.upNext.map(\.id), ["1002"])

        await s.skipToNext()
        XCTAssertEqual(s.state.nowPlaying?.id, "1002")
        await s.skipToPrevious()
        XCTAssertEqual(s.state.nowPlaying?.id, "1001")
        await s.togglePlay()
        XCTAssertFalse(fake.isPlaying)
    }

    func testRequestByTitleIsResolvedFromCatalog() async {
        let (s, _) = session()
        _ = await s.add(MusicSong(title: "海沿いの道", artist: "テイラーズ"))
        XCTAssertEqual(s.state.nowPlaying?.id, "1002", "id の無いリクエストは曲名 + アーティストで探す")
    }

    func testReceivePendingThenAcceptOrReject() async throws {
        let (s, _) = session()
        let ack = await s.receive(FakeMusicPlayback.catalog[2], from: "花子", autoAccept: false)
        XCTAssertEqual(ack, .init(ok: true, added: 1))
        XCTAssertNil(s.state.nowPlaying, "承認するまで入れない")
        XCTAssertEqual(s.inbox.pending.count, 1)
        XCTAssertEqual(s.notice?.contains("花子さんから"), true)

        let dup = await s.receive(FakeMusicPlayback.catalog[2], from: "太郎", autoAccept: false)
        XCTAssertFalse(dup.ok)

        await s.accept(try XCTUnwrap(s.inbox.pending.first).id)
        XCTAssertEqual(s.state.nowPlaying?.id, "1003")
        XCTAssertTrue(s.inbox.pending.isEmpty)

        _ = await s.receive(FakeMusicPlayback.catalog[3], from: "花子", autoAccept: false)
        s.reject(try XCTUnwrap(s.inbox.pending.first).id)
        XCTAssertTrue(s.inbox.pending.isEmpty)
        XCTAssertTrue(s.state.upNext.isEmpty)
    }

    func testAutoAcceptQueuesImmediately() async {
        let (s, _) = session()
        _ = await s.add(FakeMusicPlayback.catalog[0])
        let ack = await s.receive(FakeMusicPlayback.catalog[4], from: "花子", autoAccept: true)
        XCTAssertTrue(ack.ok)
        XCTAssertTrue(s.inbox.pending.isEmpty)
        XCTAssertEqual(s.state.upNext.map(\.id), ["1005"])
        let again = await s.receive(FakeMusicPlayback.catalog[4], from: "太郎", autoAccept: true)
        XCTAssertFalse(again.ok, "再生待ちにある曲は入れない")
    }

    func testFailedAcceptGoesBackToPending() async throws {
        let (s, _) = session(strict: true)
        _ = await s.receive(MusicSong(title: "存在しない曲", artist: "誰か"), from: "花子", autoAccept: false)
        await s.accept(try XCTUnwrap(s.inbox.pending.first).id)
        XCTAssertEqual(s.inbox.pending.count, 1, "見つからなければ承認待ちに戻す")
        XCTAssertEqual(s.notice?.contains("見つかりませんでした"), true)
    }

    func testNoSubscriptionCanSearchButNotPlay() async {
        let (s, _) = session(.noSubscription)
        await s.prepare(requesting: false)
        XCTAssertFalse(s.availability.canPlay)
        if case .success(let songs) = await s.search("Samples") {
            XCTAssertEqual(songs.count, 2)
        } else {
            XCTFail("検索はできる")
        }
        let ok = await s.add(FakeMusicPlayback.catalog[0])
        XCTAssertFalse(ok)
        XCTAssertEqual(s.notice, MusicAvailability.noSubscription.message)
        let ack = await s.receive(FakeMusicPlayback.catalog[1], from: "花子", autoAccept: true)
        XCTAssertFalse(ack.ok, "契約が無ければ自動で入れられない")
    }

    func testAvailabilityMessages() {
        for a in [MusicAvailability.denied, .restricted, .noSubscription, .unavailable] {
            XCTAssertFalse((a.message ?? "").isEmpty, "\(a)")
        }
        XCTAssertNil(MusicAvailability.ready.message)
    }

    /// 部品が登録され、偽のプレーヤーで描画できる(承認待ち・再生待ちあり)。
    func testLeafRenders() async throws {
        let fake = FakeMusicPlayback()
        MusicSession.shared.player = fake
        await MusicSession.shared.prepare(requesting: false)
        _ = await MusicSession.shared.add(FakeMusicPlayback.catalog[0])
        _ = await MusicSession.shared.add(FakeMusicPlayback.catalog[1])
        _ = await MusicSession.shared.receive(FakeMusicPlayback.catalog[2], from: "花子", autoAccept: false)

        let json = #"{"type":"musicPlayer","title":"ドライブ BGM","allowRequests":true}"#
        let c = try JSONDecoder().decode(Component.self, from: Data(json.utf8))
        XCTAssertEqual(c.type, .musicPlayer)
        let build = try XCTUnwrap(ComponentRegistry.standard.builder(for: "musicPlayer"))
        let view = build(c, RenderContext(route: Route(screen: "home")))
            .frame(width: 390, height: 600)
        let host = UIHostingController(rootView: view)
        host.view.frame = CGRect(x: 0, y: 0, width: 390, height: 600)
        host.view.layoutIfNeeded()
        let size = host.sizeThatFits(in: CGSize(width: 390, height: 600))
        XCTAssertGreaterThan(size.height, 0)
        XCTAssertNotNil(ImageRenderer(content: view).uiImage)
    }
}
