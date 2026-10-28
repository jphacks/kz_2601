#if !APP_STORE_REVIEW
import XCTest
import MetamonDSL
import MetamonRenderer
@testable import Metamon

/// すれ違い通信(#425): 同意・オンの前は通信しない / 自分の記録だけ送る / 受け取りの重複除け。通信路は差し替え。
@MainActor
final class PassbyControllerTests: XCTestCase {
    private final class FakeTransport: PassbyTransport {
        var onPayload: ((NearbyPassby.Payload) -> Void)?
        var onPeers: (([PassbyPeer]) -> Void)?
        var onNotice: ((String?) -> Void)?
        var distanceSupport: PassbyDistanceSupport { .unsupported("x") }
        var started: PassbyStart?
        var stops = 0
        func start(_ start: PassbyStart) { started = start }
        func stop() { started = nil; stops += 1 }
    }

    private let json = """
    {"schemaVersion":1,"app":{"name":"すれ違い","voiceEntry":"none"},"initialScreen":"home",
     "entities":[{"id":"profile","name":"プロフィール","fields":[
       {"id":"name","name":"名前","type":"string","required":true},{"id":"hobby","name":"趣味","type":"string"},
       {"id":"memo","name":"メモ","type":"string","hidden":true}]}],
     "screens":[{"id":"home","title":"すれ違い","components":[
       {"type":"nearbyShare","id":"pb","label":"すれ違い通信","entity":"profile","mode":"passby","fields":["name","hobby","memo"],"limit":5}]}]}
    """

    func testConsentOptInExchangeAndDedupe() async throws {
        let dsl = try JSONDecoder().decode(AppDSL.self, from: Data(json.utf8))
        let appId = "passby-test-\(UUID().uuidString)"
        let rt = AppRuntime(document: AppDocument(appId: appId, version: 1, dsl: dsl), services: BuildConfig.make(environment: .mock))
        let suite = "passby-test-\(UUID().uuidString)"
        let defaults = try XCTUnwrap(UserDefaults(suiteName: suite))
        defer { defaults.removePersistentDomain(forName: suite) }
        let fake = FakeTransport()
        let c = PassbyController(transport: fake, defaults: defaults)
        c.runtime = rt
        let config = try XCTUnwrap(NearbyPassby.Config(dsl.screens[0].components[0]))

        c.attach(config)
        try await Task.sleep(for: .milliseconds(200))
        XCTAssertNil(fake.started, "オンにするまで探さない")
        c.setOptIn(true, config: config)
        XCTAssertNil(fake.started, "同意の前は探さない")
        c.agree()
        c.setOptIn(true, config: config)
        let start = try XCTUnwrap(fake.started)
        XCTAssertTrue(c.isAdvertising)
        XCTAssertEqual(start.hello.app, NearbyPassby.appTag(appId))

        _ = await rt.createRecord(entity: "profile", data: ["name": "太郎", "hobby": "釣り", "memo": "秘密"])
        let mine = start.payload()
        XCTAssertEqual(mine.records.map(\.data), [["name": "太郎", "hobby": "釣り"]], "hidden は送らない")

        let theirs = NearbyPassby.Payload(v: 1, senderId: "s2", name: "花子", entityId: "profile",
                                          records: [.init(id: "r1", data: ["name": "花子", "hobby": "読書", "memo": "x"])])
        await c.receive(theirs)
        await c.receive(theirs)
        XCTAssertEqual(rt.records(for: "profile").count, 2, "同じ相手・同じ記録は 1 回だけ")
        XCTAssertTrue(rt.records(for: "profile").allSatisfy { $0.data["memo"] != .string("x") }, "許可していない項目は保存しない")

        var changed = theirs
        changed.records[0].data["hobby"] = "映画"
        await c.receive(changed)
        XCTAssertEqual(rt.records(for: "profile").count, 2, "内容が変わったら更新")
        XCTAssertTrue(rt.records(for: "profile").contains { $0.data["hobby"] == "映画" })

        XCTAssertEqual(start.payload().records.map { $0.data["name"] }, [.string("太郎")], "受け取った記録は送り返さない")
        XCTAssertEqual(c.encounters.map(\.name), ["花子"])
        XCTAssertFalse(start.shouldExchange("s2"), "続けて交換しない")

        c.detach(config)
        XCTAssertNil(fake.started)
        XCTAssertFalse(c.isAdvertising)
    }
}
#endif
