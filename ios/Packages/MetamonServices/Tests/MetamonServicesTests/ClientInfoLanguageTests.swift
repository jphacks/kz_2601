import XCTest
@testable import MetamonServices

/// Accept-Language はアプリ内の言語設定(metamon.appLanguage)に従う。system / 未設定は端末の言語から ja / en。
final class ClientInfoLanguageTests: XCTestCase {
    func testFollowsAppLanguageSetting() throws {
        let name = "ClientInfoLanguageTests"
        let defaults = try XCTUnwrap(UserDefaults(suiteName: name))
        defer { defaults.removePersistentDomain(forName: name) }
        defaults.set("en", forKey: "metamon.appLanguage")
        XCTAssertEqual(ClientInfo.languageCode(defaults: defaults), "en")
        defaults.set("ja", forKey: "metamon.appLanguage")
        XCTAssertEqual(ClientInfo.languageCode(defaults: defaults), "ja")
        defaults.set("system", forKey: "metamon.appLanguage")
        XCTAssertTrue(["ja", "en"].contains(ClientInfo.languageCode(defaults: defaults)))
        defaults.removeObject(forKey: "metamon.appLanguage")
        XCTAssertTrue(["ja", "en"].contains(ClientInfo.languageCode(defaults: defaults)))
    }
}
