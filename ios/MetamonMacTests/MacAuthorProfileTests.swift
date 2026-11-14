import XCTest
@testable import Metamon

/// 共有の投稿に出す表示名(#3077): iOS と共有する `AuthorProfile` の正規化と保存
final class MacAuthorProfileTests: XCTestCase {
    private var savedName: String?
    private var savedAsked = false

    override func setUp() {
        super.setUp()
        savedName = UserDefaults.standard.string(forKey: AuthorProfile.nameKey)
        savedAsked = UserDefaults.standard.bool(forKey: AuthorProfile.askedKey)
    }

    override func tearDown() {
        if let savedName { UserDefaults.standard.set(savedName, forKey: AuthorProfile.nameKey) }
        else { UserDefaults.standard.removeObject(forKey: AuthorProfile.nameKey) }
        UserDefaults.standard.set(savedAsked, forKey: AuthorProfile.askedKey)
        super.tearDown()
    }

    func testNormalizedTrimsWhitespace() {
        XCTAssertEqual(AuthorProfile.normalized("  たろう \n"), "たろう")
    }

    func testNormalizedEmptyOrBlankIsNil() {
        XCTAssertNil(AuthorProfile.normalized(""))
        XCTAssertNil(AuthorProfile.normalized(" \t\n　"))
    }

    func testNormalizedCutsToMaxLength() {
        let long = String(repeating: "あ", count: AuthorProfile.maxLength + 10)
        XCTAssertEqual(AuthorProfile.normalized(long)?.count, AuthorProfile.maxLength)
    }

    func testNameRoundTripAndClear() {
        AuthorProfile.name = "  ハナ "
        XCTAssertEqual(AuthorProfile.name, "ハナ")
        AuthorProfile.name = "   "
        XCTAssertNil(AuthorProfile.name)
        AuthorProfile.name = nil
        XCTAssertNil(AuthorProfile.name)
    }
}
