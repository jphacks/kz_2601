import XCTest
@testable import MetamonDSL

/// 曲のリクエストの同じ曲の判定(`MusicSong.isSameSong`)と検索の語(`MusicLinks.searchTerm`)。
final class MusicSongMatchTests: XCTestCase {
    // MARK: - isSameSong

    func testSameIDWins() {
        let a = MusicSong(id: "123", title: "Lemon", artist: "米津玄師")
        XCTAssertTrue(a.isSameSong(as: MusicSong(id: "123", title: "別の曲名", artist: "別の人")))
        XCTAssertFalse(a.isSameSong(as: MusicSong(id: "456", title: "Lemon", artist: "米津玄師")))
    }

    func testIgnoresCaseAndSurroundingWhitespace() {
        let a = MusicSong(title: "Lemon", artist: "Kenshi Yonezu")
        XCTAssertTrue(a.isSameSong(as: MusicSong(title: "  lemon\n", artist: "KENSHI YONEZU ")))
        // 片方だけ id があれば曲名 + アーティストで比べる
        XCTAssertTrue(a.isSameSong(as: MusicSong(id: "123", title: "LEMON", artist: "kenshi yonezu")))
    }

    func testDifferentArtistIsDifferentSong() {
        let a = MusicSong(title: "Lemon", artist: "Kenshi Yonezu")
        XCTAssertFalse(a.isSameSong(as: MusicSong(title: "Lemon", artist: "Someone Else")))
        XCTAssertFalse(a.isSameSong(as: MusicSong(title: "Lemon", artist: "")))
        XCTAssertFalse(a.isSameSong(as: MusicSong(title: "Lemonade", artist: "Kenshi Yonezu")))
    }

    func testDoesNotFoldWidthOrFeaturing() {
        // 今の決まり: 全角・半角や feat. の有無はそろえない(別の曲として扱う)
        let a = MusicSong(title: "Lemon", artist: "Kenshi Yonezu")
        XCTAssertFalse(a.isSameSong(as: MusicSong(title: "Ｌｅｍｏｎ", artist: "Kenshi Yonezu")))
        XCTAssertFalse(a.isSameSong(as: MusicSong(title: "Lemon", artist: "Kenshi Yonezu feat. X")))
        // 中の空白の数もそろえない
        XCTAssertFalse(a.isSameSong(as: MusicSong(title: "Lemon", artist: "Kenshi  Yonezu")))
    }

    // MARK: - searchTerm

    func testSearchTermJoinsTitleAndArtist() {
        XCTAssertEqual(MusicLinks.searchTerm(MusicSong(title: "Lemon", artist: "米津玄師")), "Lemon 米津玄師")
        XCTAssertEqual(MusicLinks.searchTerm(MusicSong(title: " Lemon ", artist: " 米津玄師 ")), "Lemon 米津玄師")
    }

    func testSearchTermWithoutArtistOrTitle() {
        XCTAssertEqual(MusicLinks.searchTerm(MusicSong(title: "Lemon", artist: "")), "Lemon")
        XCTAssertEqual(MusicLinks.searchTerm(MusicSong(title: "Lemon", artist: "   ")), "Lemon")
        XCTAssertEqual(MusicLinks.searchTerm(MusicSong(title: "", artist: "米津玄師")), "米津玄師")
        XCTAssertEqual(MusicLinks.searchTerm(MusicSong(title: " ", artist: "")), "")
    }
}
