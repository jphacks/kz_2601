import XCTest
import MetamonRenderer
@testable import Metamon

/// #279: 識別子は「詳細(サポート用)」にまとめ、アカウント欄は状態だけ。
@MainActor
final class SupportInfoTests: XCTestCase {
    private let info = SupportInfo(uid: "abc123", voiceModel: "openai_whisper-base", aiEndpoint: "ai.example.com",
                                   appConfigEndpoint: "appconfig.example.com", appVersion: "1.2", build: "34", appRevision: 5)
    private var saved: AppLanguage = .system

    override func setUp() {
        saved = LanguagePreference.shared.setting
        LanguagePreference.shared.setting = .ja
    }

    override func tearDown() { LanguagePreference.shared.setting = saved }

    func testRowsHoldEveryIdentifier() {
        let values = info.rows.map(\.value)
        XCTAssertEqual(values.first, "abc123", "uid は先頭(問い合わせで最初に聞く)")
        for v in ["openai_whisper-base", "ai.example.com", "appconfig.example.com", "1.2 (34)", "v5"] {
            XCTAssertTrue(values.contains(v), "\(v) が無い")
        }
    }

    func testNoUIDRowWhenSignedOut() {
        var signedOut = info
        signedOut.uid = nil
        XCTAssertEqual(signedOut.rows.count, info.rows.count - 1)
        XCTAssertFalse(signedOut.clipboardText.contains("abc123"))
    }

    func testClipboardHasOneLinePerRow() {
        let lines = info.clipboardText.split(separator: "\n")
        XCTAssertEqual(lines.count, info.rows.count)
        XCTAssertTrue(lines[0].hasSuffix(": abc123"))
    }

    /// #973: Mac は音声モデル・接続先・アプリの版を持たない。その行は出さず、バージョンだけ残る。
    func testMacShapeShowsOnlyVersion() {
        let mac = SupportInfo(uid: nil, voiceModel: nil, aiEndpoint: nil, appConfigEndpoint: nil,
                              appVersion: "1.2", build: "34", appRevision: nil)
        XCTAssertEqual(mac.rows.map(\.value), ["1.2 (34)"])
        XCTAssertEqual(mac.clipboardText, "バージョン: 1.2 (34)")
    }

    func testAccountStatusHasNoIdentifier() {
        XCTAssertEqual(SupportInfo.accountStatus(isAnonymous: true), "匿名で利用中")
        XCTAssertEqual(SupportInfo.accountStatus(isAnonymous: false), "Apple でサインイン中")
        LanguagePreference.shared.setting = .en
        XCTAssertEqual(SupportInfo.accountStatus(isAnonymous: true), "Using anonymously")
        XCTAssertEqual(info.rows.first?.label, "User ID")
    }
}
