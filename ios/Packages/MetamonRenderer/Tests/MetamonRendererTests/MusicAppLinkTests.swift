import XCTest
import MetamonDSL
@testable import MetamonRenderer

/// Mac の「ミュージック アプリで開く」(#1348): 出す時・URL・文言。
final class MusicAppLinkTests: XCTestCase {
    private var savedLanguage: AppLanguage = .system

    override func setUp() {
        savedLanguage = LanguagePreference.shared.setting
        LanguagePreference.shared.setting = .ja
    }

    override func tearDown() { LanguagePreference.shared.setting = savedLanguage }

    func testOfferedOnlyWhenCannotPlayOnMac() {
        #if os(macOS)
        XCTAssertTrue(MusicAppLink.offered(for: .denied))
        XCTAssertTrue(MusicAppLink.offered(for: .noSubscription))
        XCTAssertTrue(MusicAppLink.offered(for: .unavailable))
        XCTAssertFalse(MusicAppLink.offered(for: .restricted), "制限されている時は出さない")
        #else
        XCTAssertFalse(MusicAppLink.offered(for: .noSubscription), "iOS は今までどおり出さない")
        XCTAssertFalse(MusicAppLink.offered(for: .denied))
        #endif
        XCTAssertFalse(MusicAppLink.offered(for: .ready))
        XCTAssertFalse(MusicAppLink.offered(for: .notDetermined))
    }

    func testAppURLUsesMusicScheme() {
        XCTAssertEqual(MusicAppLink.appURL(for: MusicSong(id: "1001", title: "夜明けのドライブ", artist: "テイラーズ"))?.absoluteString,
                       "music://music.apple.com/song/1001")
        let search = MusicAppLink.appURL(for: MusicSong(title: "Night Radio", artist: "The Samples"))
        XCTAssertEqual(search?.scheme, "music")
        XCTAssertEqual(search?.path, "/search")
        XCTAssertEqual(MusicAppLink.appURL(for: nil)?.absoluteString, "music://", "曲が無ければアプリだけ開く")
        XCTAssertNil(MusicAppLink.appURL(for: MusicSong(title: " ", artist: "")))
    }

    func testLabel() {
        XCTAssertEqual(MusicAppLink.label(for: MusicSong(title: "a", artist: "b")), "ミュージック アプリで開く")
        XCTAssertEqual(MusicAppLink.label(for: nil), "ミュージック アプリを開く")
        LanguagePreference.shared.setting = .en
        XCTAssertEqual(MusicAppLink.label(for: MusicSong(title: "a", artist: "b")), "Open in Music")
        XCTAssertEqual(MusicAppLink.label(for: nil), "Open Music")
    }
}
