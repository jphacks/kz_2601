import XCTest
import MetamonServices
@testable import Metamon

/// アプリ内のお知らせ(#333)。UpdateGate がバナーに出すか・閉じた id を覚えるか・更新画面の間は隠すか。
@MainActor
final class ServiceNoticeGateTests: XCTestCase {
    private struct StubClient: ClientConfigClient {
        let config: ClientConfig
        func fetch() async throws -> ClientConfig { config }
    }

    private var suite = ""
    private var defaults: UserDefaults!

    override func setUp() {
        super.setUp()
        suite = "ServiceNoticeGateTests.\(UUID().uuidString)"
        defaults = UserDefaults(suiteName: suite)
    }

    override func tearDown() {
        defaults.removePersistentDomain(forName: suite)
        super.tearDown()
    }

    private func gate(_ config: ClientConfig, build: String = "10") -> UpdateGate {
        UpdateGate(checker: UpdateChecker(client: StubClient(config: config), build: build), dismissals: NoticeDismissals(defaults: defaults))
    }

    private let notice = ServiceNotice(id: "2026-11-02-ai", severity: .warning, message: .init(ja: "止めています", en: "Paused"))

    func testShowsNoticeAndRemembersDismissal() async {
        let g = gate(ClientConfig(notice: notice))
        await g.check()
        XCTAssertEqual(g.visibleNotice?.id, "2026-11-02-ai")
        g.dismissServiceNotice()
        XCTAssertNil(g.visibleNotice)

        let relaunched = gate(ClientConfig(notice: notice))
        await relaunched.check()
        XCTAssertNil(relaunched.visibleNotice, "閉じた id は起動し直しても出さない")

        var changed = notice
        changed.id = "2026-11-02-ai-2"
        let next = gate(ClientConfig(notice: changed))
        await next.check()
        XCTAssertEqual(next.visibleNotice?.id, "2026-11-02-ai-2", "新しい id は出す")
    }

    func testNoticeDisappearsWhenServerClearsIt() async {
        let g = gate(ClientConfig(notice: notice))
        await g.check()
        XCTAssertNotNil(g.visibleNotice)
        let cleared = gate(ClientConfig())
        await cleared.check()
        XCTAssertNil(cleared.visibleNotice)
    }

    func testHiddenBehindRequiredUpdate() async {
        let g = gate(ClientConfig(minIOSBuild: 20, notice: notice), build: "10")
        await g.check()
        XCTAssertTrue(g.isBlocking)
        XCTAssertNil(g.visibleNotice, "閉じられない更新画面の上には出さない")
    }

    func testOutOfRangeBuildSeesNothing() async {
        var ranged = notice
        ranged.maxIOSBuild = 9
        let g = gate(ClientConfig(notice: ranged), build: "10")
        await g.check()
        XCTAssertNil(g.visibleNotice)
    }
}
