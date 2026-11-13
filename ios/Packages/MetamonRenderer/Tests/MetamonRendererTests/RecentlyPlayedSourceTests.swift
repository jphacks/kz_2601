import XCTest
import MetamonDSL
@testable import MetamonRenderer

/// 最近聞いた曲の読み取り口(#1988): 状態ごとの返り方と上限。
@MainActor
final class RecentlyPlayedSourceTests: XCTestCase {
    private func songs(_ n: Int) -> [MusicSong] {
        (1...n).map { MusicSong(id: "\($0)", title: "曲\($0)", artist: "A") }
    }

    func testReadyReturnsNewestFirst() async throws {
        let got = try await FakeRecentlyPlayed().recentlyPlayed(limit: 3)
        XCTAssertEqual(got, Array(FakeRecentlyPlayed.history.prefix(3)))
        XCTAssertEqual(got.first?.id, "1005")
    }

    func testNoSubscriptionCanStillRead() async throws {
        let got = try await FakeRecentlyPlayed(state: .noSubscription).recentlyPlayed(limit: 2)
        XCTAssertEqual(got.count, 2)
    }

    func testBlockedStatesThrowUnavailable() async {
        for state in [MusicAvailability.notDetermined, .denied, .restricted, .unavailable] {
            do {
                _ = try await FakeRecentlyPlayed(state: state).recentlyPlayed(limit: 5)
                XCTFail("\(state) は読めない")
            } catch {
                XCTAssertEqual(error as? MusicPlaybackError, .unavailable(state))
            }
        }
    }

    func testRequestingPermissionMakesNotDeterminedReadable() async throws {
        let fake = FakeRecentlyPlayed(state: .notDetermined)
        let state = await fake.availability(requesting: true)
        XCTAssertEqual(state, .ready)
        let got = try await fake.recentlyPlayed(limit: 1)
        XCTAssertEqual(got.count, 1)
    }

    func testLimitIsCappedAtMax() async throws {
        let fake = FakeRecentlyPlayed(items: songs(40))
        let got = try await fake.recentlyPlayed(limit: 100)
        XCTAssertEqual(got.count, RecentlyPlayed.maxLimit)
        XCTAssertEqual(RecentlyPlayed.maxLimit, 25)
        XCTAssertEqual(fake.lastLimit, 25)
    }

    func testZeroOrNegativeLimitReturnsEmptyEvenWhenBlocked() async throws {
        let empty0 = try await FakeRecentlyPlayed(state: .denied).recentlyPlayed(limit: 0)
        let emptyNeg = try await FakeRecentlyPlayed().recentlyPlayed(limit: -3)
        XCTAssertTrue(empty0.isEmpty && emptyNeg.isEmpty)
    }

    func testFewerItemsThanLimit() async throws {
        let got = try await FakeRecentlyPlayed(items: songs(2)).recentlyPlayed(limit: 10)
        XCTAssertEqual(got.count, 2)
    }

    func testCanReadRecentlyPlayed() {
        XCTAssertTrue(MusicAvailability.ready.canReadRecentlyPlayed)
        XCTAssertTrue(MusicAvailability.noSubscription.canReadRecentlyPlayed)
        XCTAssertFalse(MusicAvailability.denied.canReadRecentlyPlayed)
        XCTAssertFalse(MusicAvailability.notDetermined.canReadRecentlyPlayed)
    }

    /// 実体も許可が無ければ MusicKit を呼ばずに `unavailable` を投げる(シミュレータ・許可なしで落ちない)。
    func testLiveSourceThrowsWithoutCallingMusicKitWhenBlocked() async {
        let live = AppleRecentlyPlayed(authorization: { _ in .denied })
        do {
            _ = try await live.recentlyPlayed(limit: 5)
            XCTFail("denied は読めない")
        } catch {
            XCTAssertEqual(error as? MusicPlaybackError, .unavailable(.denied))
        }
        let empty = try? await live.recentlyPlayed(limit: 0)
        XCTAssertEqual(empty, [])
    }

    /// 許可だけで決める(契約は読み込まない)。
    func testLiveAvailabilityUsesAuthorizationOnly() {
        XCTAssertEqual(AppleRecentlyPlayed.availability(for: .authorized), .ready)
        XCTAssertEqual(AppleRecentlyPlayed.availability(for: .notDetermined), .notDetermined)
        XCTAssertEqual(AppleRecentlyPlayed.availability(for: .denied), .denied)
        XCTAssertEqual(AppleRecentlyPlayed.availability(for: .restricted), .restricted)
    }

    /// 読み込み失敗は MusicKit のエラーでなく `unavailable` の状態に寄せる。
    func testFailureStateMapsToNoSubscriptionOrUnavailable() {
        XCTAssertEqual(AppleRecentlyPlayed.failureState(canPlayCatalog: false), .noSubscription)
        XCTAssertEqual(AppleRecentlyPlayed.failureState(canPlayCatalog: true), .unavailable)
        XCTAssertEqual(AppleRecentlyPlayed.failureState(canPlayCatalog: nil), .unavailable)
    }

    func testArtworkKeepsOnlyHTTPS() {
        XCTAssertEqual(AppleRecentlyPlayed.httpsArtwork(URL(string: "https://a.example/x.jpg")), "https://a.example/x.jpg")
        XCTAssertNil(AppleRecentlyPlayed.httpsArtwork(URL(string: "http://a.example/x.jpg")))
        XCTAssertNil(AppleRecentlyPlayed.httpsArtwork(URL(string: "musicKit://artwork/1")))
        XCTAssertNil(AppleRecentlyPlayed.httpsArtwork(nil))
    }
}
