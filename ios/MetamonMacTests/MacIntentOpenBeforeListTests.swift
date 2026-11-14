import XCTest
import MetamonDSL
import MetamonServices
@testable import Metamon

/// 起動直後(一覧を読み終える前)に来たウィジェット・ショートカットの「開く」を、読み終えてから開く(#1970)
@MainActor
final class MacIntentOpenBeforeListTests: XCTestCase {
    /// 一覧を返すのが遅い appconfig
    private actor SlowList: AppConfigClient {
        let apps: [AppSummary]
        let delay: Duration
        init(apps: [AppSummary], delay: Duration) {
            self.apps = apps
            self.delay = delay
        }

        func listApps() async throws -> [AppSummary] {
            try? await Task.sleep(for: delay)
            return apps
        }

        func fetchDocument(appId: String) async throws -> AppDocument { throw URLError(.notConnectedToInternet) }
        func listVersions(appId: String) async throws -> [VersionSummary] { [] }
        func restore(appId: String, toVersion version: Int) async throws -> AppDocument { throw URLError(.badServerResponse) }
        func list(appId: String, entity: String) async throws -> [Record] { [] }
        func create(appId: String, entity: String, data: [String: JSONValue]) async throws -> Record { Record(id: "x", data: data) }
        func update(appId: String, entity: String, id: String, data: [String: JSONValue]) async throws -> Record { Record(id: id, data: data) }
        func delete(appId: String, entity: String, id: String) async throws {}
    }

    private func makeHost(serverApps: [AppSummary], delay: Duration = .milliseconds(300)) -> (IntentHost, MacAppLibrary) {
        let library = MacAppLibrary(store: nil, server: SlowList(apps: serverApps, delay: delay))
        let host = IntentHost(server: { nil })
        host.register(library: library, bundled: [])
        return (host, library)
    }

    private func receiveWidget(_ appId: String, host: IntentHost) async {
        XCTAssertTrue(MacWidgetLinkReceiver.receive(MacWidgetLink.url(appId: appId), host: host))
        for _ in 0..<50 where host.pending == nil { await Task.yield() }
    }

    func testWidgetOpenWaitsForSlowListThenOpens() async throws {
        let (host, library) = makeHost(serverApps: [AppSummary(id: "srv", name: "サーバーのアプリ", currentVersion: 2)])
        await receiveWidget("srv", host: host)

        let refresh = Task { await library.refreshServerApps() }
        await Task.yield()
        XCTAssertFalse(library.isServerListSettled, "一覧を読んでいる途中")
        XCTAssertNil(host.resolve(among: library.createdIds, listSettled: library.isServerListSettled), "読み終えるまで待つ")
        XCTAssertEqual(host.pending, .open(appId: "srv"), "前に出すだけで捨てない")

        await refresh.value
        XCTAssertTrue(library.isServerListSettled)
        XCTAssertEqual(host.resolve(among: library.createdIds, listSettled: library.isServerListSettled), .select("srv"))
        XCTAssertNil(host.pending, "1 回だけ開く")
    }

    func testDeletedTargetIsReportedAfterListLoads() async throws {
        let (host, library) = makeHost(serverApps: [AppSummary(id: "other", name: "別のアプリ", currentVersion: 2)],
                                       delay: .milliseconds(50))
        await receiveWidget("gone", host: host)
        XCTAssertNil(host.resolve(among: library.createdIds, listSettled: library.isServerListSettled))

        await library.refreshServerApps()
        XCTAssertEqual(host.resolve(among: library.createdIds, listSettled: library.isServerListSettled), .missing("gone"),
                       "消したアプリは一覧を出して知らせる")
        XCTAssertNil(host.pending)
    }

    func testGivesUpWhenListNeverLoads() async throws {
        var clock = Date(timeIntervalSince1970: 1_000)
        let host = IntentHost(server: { nil })
        host.now = { clock }
        await host.request(.open(appId: "srv"))
        XCTAssertNil(host.resolve(among: [], listSettled: false))
        clock += IntentHost.openLifetime
        XCTAssertEqual(host.resolve(among: [], listSettled: false), .missing("srv"), "待つ長さを過ぎたら後から急に開かない")
    }

    func testTalkIsNotHeldForList() async throws {
        let host = IntentHost(server: { nil })
        await host.request(.talk)
        XCTAssertEqual(host.resolve(among: [], listSettled: false), .talk)
    }

    func testSignedOutLibraryCountsAsSettled() async throws {
        let (host, library) = makeHost(serverApps: [])
        library.resetAfterWipe() // サインアウトの後: `.idle` のまま取り直さない
        XCTAssertFalse(library.isServerListSettled)
        XCTAssertTrue(library.isServerListSettled(signedIn: false), "サインインしていなければ一覧を待たない")
        XCTAssertFalse(library.isServerListSettled(signedIn: true))
        await host.request(.open(appId: "gone"))
        XCTAssertEqual(host.resolve(among: library.createdIds, listSettled: library.isServerListSettled(signedIn: false)),
                       .missing("gone"), "30 秒待たずにすぐ知らせる")
    }

    func testRepeatedOpenRestartsWait() async throws {
        var clock = Date(timeIntervalSince1970: 1_000)
        let host = IntentHost(server: { nil })
        host.now = { clock }
        await host.request(.open(appId: "srv"))
        let first = host.requestSerial
        XCTAssertEqual(host.openWaitRemaining, IntentHost.openLifetime)
        clock += 20
        await host.request(.open(appId: "srv")) // 同じ「開く」をもう一度
        XCTAssertNotEqual(host.requestSerial, first, "同じ中身でも待ちを測り直すきっかけになる")
        XCTAssertEqual(host.openWaitRemaining, IntentHost.openLifetime, "待ち時間は測り直す")
        clock += 15
        XCTAssertNil(host.resolve(among: [], listSettled: false), "最初の受け取りから 30 秒でも、まだ待つ")
        XCTAssertEqual(host.openWaitRemaining, IntentHost.openLifetime - 15)
        clock += IntentHost.openLifetime - 15
        XCTAssertEqual(host.openWaitRemaining, 0)
        XCTAssertEqual(host.resolve(among: [], listSettled: false), .missing("srv"))
        XCTAssertNil(host.openWaitRemaining, "決まったら待たない")
    }

    func testTalkHasNoOpenWait() async {
        let host = IntentHost(server: { nil })
        await host.request(.talk)
        XCTAssertNil(host.openWaitRemaining)
    }

    func testLocalOnlyLibraryIsAlwaysSettled() {
        XCTAssertTrue(MacAppLibrary(store: nil).isServerListSettled, "サーバーの無い時は端末の一覧だけで決める")
    }
}
