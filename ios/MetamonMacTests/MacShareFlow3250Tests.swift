import XCTest
@testable import Metamon

/// #3250: 共有で使う表示名の覚え方(`MacSharedDisplayName`)。UserDefaults の鍵は最後に元へ戻す
final class MacShareFlow3250Tests: XCTestCase {
    private var saved: Any?

    override func setUp() {
        saved = UserDefaults.standard.object(forKey: MacSharedDisplayName.key)
        UserDefaults.standard.removeObject(forKey: MacSharedDisplayName.key)
    }

    override func tearDown() {
        if let saved { UserDefaults.standard.set(saved, forKey: MacSharedDisplayName.key) }
        else { UserDefaults.standard.removeObject(forKey: MacSharedDisplayName.key) }
    }

    func testLastIsEmptyBeforeRemembering() {
        XCTAssertEqual(MacSharedDisplayName.last, "")
    }

    func testRememberThenRead() {
        MacSharedDisplayName.remember("たろう")
        XCTAssertEqual(MacSharedDisplayName.last, "たろう")
    }

    func testRememberTrimsSurroundingWhitespace() {
        MacSharedDisplayName.remember("  たろう \n")
        XCTAssertEqual(MacSharedDisplayName.last, "たろう")
    }

    func testBlankIsNotRememberedAndKeepsPrevious() {
        MacSharedDisplayName.remember("")
        XCTAssertEqual(MacSharedDisplayName.last, "")
        MacSharedDisplayName.remember("はなこ")
        MacSharedDisplayName.remember("   \t ")
        XCTAssertEqual(MacSharedDisplayName.last, "はなこ")
    }

    func testLongNameIsCutToMaxLength() {
        MacSharedDisplayName.remember(String(repeating: "あ", count: MacSharedDisplayName.maxLength + 10))
        XCTAssertEqual(MacSharedDisplayName.last.count, MacSharedDisplayName.maxLength)
    }

    /// サインアウトの掃除(`MacLocalWipe` が使う `LocalWipe.clearDefaults`)で消える。本物の UserDefaults は触らず別の suite で確かめる
    func testWipeClearsRememberedName() throws {
        XCTAssertFalse(LocalWipe.preservedKeys.contains(MacSharedDisplayName.key), "残すキーに入っていない")
        let suite = "mac-share-3250-\(UUID().uuidString)"
        let defaults = try XCTUnwrap(UserDefaults(suiteName: suite))
        defer { defaults.removePersistentDomain(forName: suite) }
        defaults.set("たろう", forKey: MacSharedDisplayName.key)
        LocalWipe.clearDefaults(defaults, domain: suite)
        XCTAssertNil(defaults.string(forKey: MacSharedDisplayName.key))
    }
}
