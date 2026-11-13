import XCTest
@testable import MetamonServices

/// AI へ送る同意の保存(#246)。テスト専用の UserDefaults で、同意・取り消し・聞いた印・読み直しを確かめる。
@MainActor
final class AIConsentStoreTests: XCTestCase {
    private var suite: String!
    private var defaults: UserDefaults!

    override func setUp() {
        super.setUp()
        suite = "AIConsentStoreTests.\(UUID().uuidString)"
        defaults = UserDefaults(suiteName: suite)
    }

    override func tearDown() {
        defaults.removePersistentDomain(forName: suite)
        super.tearDown()
    }

    func testInitialStateIsNotGranted() {
        let store = AIConsentStore(defaults: defaults)
        XCTAssertFalse(store.isGranted)
        XCTAssertNil(store.grantedAt)
        XCTAssertFalse(store.hasBeenPrompted)
        XCTAssertFalse(store.needsReconsent)
        XCTAssertTrue(store.shouldPromptOnLaunch, "初回は起動時に出す")
    }

    func testGrantStoresVersionAndDate() {
        let store = AIConsentStore(defaults: defaults)
        let now = Date(timeIntervalSince1970: 1_800_000_000)
        store.grant(now: now)
        XCTAssertTrue(store.isGranted)
        XCTAssertEqual(store.grantedAt, now)
        XCTAssertEqual(defaults.integer(forKey: AIConsentStore.versionKey), AIConsentStore.currentVersion)
        XCTAssertEqual(defaults.object(forKey: AIConsentStore.dateKey) as? Date, now)
        XCTAssertFalse(store.shouldPromptOnLaunch)
    }

    func testRevokeClearsAndIsNotReconsent() {
        let store = AIConsentStore(defaults: defaults)
        store.grant()
        store.revoke()
        XCTAssertFalse(store.isGranted)
        XCTAssertNil(store.grantedAt)
        XCTAssertNil(defaults.object(forKey: AIConsentStore.versionKey))
        XCTAssertNil(defaults.object(forKey: AIConsentStore.dateKey))
        XCTAssertFalse(store.needsReconsent, "取り消した人は版が古い人に含めない")
    }

    func testMarkPromptedSavesFlagAndVersion() {
        let store = AIConsentStore(defaults: defaults)
        store.markPrompted()
        XCTAssertTrue(store.hasBeenPrompted)
        XCTAssertEqual(defaults.integer(forKey: AIConsentStore.promptedVersionKey), AIConsentStore.currentVersion)
        XCTAssertFalse(store.shouldPromptOnLaunch, "「あとで」の人には毎回は出さない")
    }

    func testNewInstanceReadsSavedConsent() {
        let now = Date(timeIntervalSince1970: 1_800_000_000)
        AIConsentStore(defaults: defaults).grant(now: now)
        let again = AIConsentStore(defaults: defaults)
        XCTAssertTrue(again.isGranted)
        XCTAssertEqual(again.grantedAt, now)
    }

    func testReloadFollowsDirectlyErasedDefaults() {
        let store = AIConsentStore(defaults: defaults)
        store.grant()
        defaults.removeObject(forKey: AIConsentStore.versionKey)
        defaults.removeObject(forKey: AIConsentStore.dateKey)
        XCTAssertTrue(store.isGranted, "reload 前は古い状態のまま")
        store.reload()
        XCTAssertFalse(store.isGranted)
        XCTAssertNil(store.grantedAt)
    }

    func testOldVersionNeedsReconsentAndPromptsOncePerVersion() {
        defaults.set(AIConsentStore.currentVersion - 1, forKey: AIConsentStore.versionKey)
        defaults.set(true, forKey: AIConsentStore.promptedKey)
        defaults.set(AIConsentStore.currentVersion - 1, forKey: AIConsentStore.promptedVersionKey)
        let store = AIConsentStore(defaults: defaults)
        XCTAssertFalse(store.isGranted)
        XCTAssertNil(store.grantedAt)
        XCTAssertTrue(store.needsReconsent)
        XCTAssertTrue(store.shouldPromptOnLaunch)
        store.markPrompted()
        XCTAssertFalse(store.shouldPromptOnLaunch, "その版では 1 回だけ")
    }

    func testIsGrantedInDefaultsBoundary() {
        XCTAssertFalse(AIConsentStore.isGranted(in: defaults))
        defaults.set(AIConsentStore.currentVersion - 1, forKey: AIConsentStore.versionKey)
        XCTAssertFalse(AIConsentStore.isGranted(in: defaults))
        defaults.set(AIConsentStore.currentVersion, forKey: AIConsentStore.versionKey)
        XCTAssertTrue(AIConsentStore.isGranted(in: defaults))
        defaults.set(AIConsentStore.currentVersion + 1, forKey: AIConsentStore.versionKey)
        XCTAssertTrue(AIConsentStore.isGranted(in: defaults), "新しい版は同意済みのまま")
    }

    func testResetForTestingReturnsToFirstLaunch() {
        let store = AIConsentStore(defaults: defaults)
        store.grant()
        store.markPrompted()
        store.resetForTesting()
        XCTAssertFalse(store.isGranted)
        XCTAssertFalse(store.hasBeenPrompted)
        XCTAssertNil(defaults.object(forKey: AIConsentStore.promptedVersionKey))
        XCTAssertTrue(store.shouldPromptOnLaunch)
    }
}
