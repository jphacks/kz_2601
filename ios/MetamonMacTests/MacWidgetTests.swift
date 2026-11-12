import XCTest
import MetamonDSL
@testable import Metamon

/// デスクトップのウィジェット(#984): 押した時の URL・本体の受け取り・スナップショットに入れる appId
@MainActor
final class MacWidgetTests: XCTestCase {
    private func kakeibo() throws -> AppDSL {
        let url = try XCTUnwrap(Bundle(for: MacAppRuntime.self).url(forResource: "kakeibo", withExtension: "json"))
        return try AppDSL.load(contentsOf: url)
    }

    func testLinkRoundTrip() throws {
        let url = MacWidgetLink.url(appId: "app-1")
        XCTAssertEqual(url.absoluteString, "tailorit://widget?app=app-1")
        XCTAssertEqual(MacWidgetLink.appId(from: url), "app-1")
    }

    func testLinkWithoutAppOnlyBringsAppForward() throws {
        let url = MacWidgetLink.url(appId: nil)
        XCTAssertEqual(url.absoluteString, "tailorit://widget")
        XCTAssertTrue(MacWidgetLink.isWidget(url))
        XCTAssertNil(MacWidgetLink.appId(from: url))
    }

    func testOtherLinksAreNotWidget() throws {
        XCTAssertFalse(MacWidgetLink.isWidget(try XCTUnwrap(URL(string: "tailorit://import/abc"))))
        XCTAssertFalse(MacWidgetLink.isWidget(try XCTUnwrap(URL(string: "metamon://widget?app=a"))))
        XCTAssertNil(MacWidgetLink.appId(from: try XCTUnwrap(URL(string: "https://tailor-it.app/s/abc"))))
    }

    func testReceiverOpensAppThroughIntentHost() async throws {
        let host = IntentHost(server: { nil })
        XCTAssertTrue(MacWidgetLinkReceiver.receive(MacWidgetLink.url(appId: "app-1"), host: host))
        for _ in 0..<50 where host.pending == nil { await Task.yield() }
        XCTAssertEqual(host.take(), .open(appId: "app-1"))
        XCTAssertFalse(MacWidgetLinkReceiver.receive(try XCTUnwrap(URL(string: "tailorit://import/abc")), host: host))
    }

    func testSnapshotCarriesAppId() throws {
        let dsl = try kakeibo()
        let snapshot = WidgetSnapshotWriter.makeSnapshot(dsl: dsl, records: [:], previous: nil, appId: "app-1")
        XCTAssertEqual(snapshot.appId, "app-1")
        XCTAssertEqual(snapshot.appName, dsl.app.name)
        XCTAssertFalse(snapshot.stats.isEmpty, "家計簿の stat が入る")
        // appId が違えば中身が違う(書き直してウィジェットを描き直す)
        var other = snapshot
        other.appId = "app-2"
        XCTAssertFalse(snapshot.hasSameContent(as: other))
    }

    func testOldSnapshotWithoutAppIdStillDecodes() throws {
        let json = #"{"appName":"家計簿","stats":[],"updatedAt":0}"#
        let snapshot = try JSONDecoder().decode(WidgetSnapshot.self, from: Data(json.utf8))
        XCTAssertNil(snapshot.appId)
    }
}
