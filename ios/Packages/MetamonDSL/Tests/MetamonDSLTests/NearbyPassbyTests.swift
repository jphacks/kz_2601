import XCTest
@testable import MetamonDSL

/// すれ違い通信(nearbyShare mode:passby・#425)の送るデータ・上限・重複除け・同意。通信を通さずに検証する。
final class NearbyPassbyTests: XCTestCase {
    static let json = """
    {"schemaVersion":1,"app":{"name":"すれ違い","voiceEntry":"none"},"initialScreen":"home",
     "entities":[{"id":"profile","name":"プロフィール","fields":[
       {"id":"name","name":"名前","type":"string","required":true},
       {"id":"hobby","name":"趣味","type":"string"},
       {"id":"age","name":"年齢","type":"integer"},
       {"id":"secret","name":"メモ","type":"string","hidden":true},
       {"id":"photo","name":"写真","type":"photo"},
       {"id":"len","name":"文字数","type":"integer","computed":{"op":"add","args":[{"ref":"age"},1]}}]}],
     "screens":[{"id":"home","title":"すれ違い","components":[
       {"type":"nearbyShare","id":"pb","label":"すれ違い通信","entity":"profile","mode":"passby",
        "fields":["name","hobby","age","secret","photo","len"],"limit":2,"distance":true},
       {"type":"nearbyShare","label":"送る","entity":"profile"}]}]}
    """

    private func fixture() throws -> (AppDSL, Entity, NearbyPassby.Config) {
        let dsl = try JSONDecoder().decode(AppDSL.self, from: Data(Self.json.utf8))
        let entity = try XCTUnwrap(dsl.entity("profile"))
        let config = try XCTUnwrap(NearbyPassby.Config(dsl.screens[0].components[0]))
        return (dsl, entity, config)
    }

    func testConfigOnlyForPassby() throws {
        let (dsl, _, config) = try fixture()
        XCTAssertEqual(config.componentId, "pb")
        XCTAssertEqual(config.limit, 2)
        XCTAssertTrue(config.distance)
        XCTAssertNil(NearbyPassby.Config(dsl.screens[0].components[1]), "send の nearbyShare は対象外")
        XCTAssertEqual(NearbyPassby.Config(componentId: "x", entityId: "e", label: "", fields: [], limit: 99).limit, 20)
        XCTAssertEqual(NearbyPassby.Config(componentId: "x", entityId: "e", label: "", fields: [], limit: 0).limit, 1)
    }

    func testAllowedFieldsDropHiddenMediaAndComputed() throws {
        let (_, entity, config) = try fixture()
        XCTAssertEqual(NearbyPassby.allowedFields(config, in: entity).map(\.id), ["name", "hobby", "age"])
    }

    func testPayloadSendsOnlyMyNewestRecordsWithListedFields() throws {
        let (_, entity, config) = try fixture()
        var ledger = NearbyPassby.Ledger()
        ledger.commit(NearbyPassby.Ledger.key(sender: "s2", id: "r9"), ["name": "花子"])
        let records = [
            Record(id: "old", data: ["name": "古い"], createdAt: "2026-10-01T00:00:00Z"),
            Record(id: "new", data: ["name": "太郎", "hobby": "釣り", "secret": "x", "photo": "data:..", "len": 2, "age": .null],
                   createdAt: "2026-10-28T00:00:00Z"),
            Record(id: "got", data: ["name": "花子"], createdAt: "2026-10-29T00:00:00Z"),              // 受け取った記録
            Record(id: "theirs", data: ["name": "他人"], createdAt: "2026-10-30T00:00:00Z", author: "u2"), // 他人の共有レコード
            Record(id: "mine", data: ["name": "自分"], createdAt: "2026-10-27T00:00:00Z", author: "me"),
        ]
        let p = NearbyPassby.makePayload(senderId: "s1", name: "太郎", entity: entity, config: config, records: records, ledger: ledger, me: "me")
        XCTAssertEqual(p.records.map(\.id), ["new", "mine"], "新しい順に limit 件・受け取った/他人の記録は除く")
        XCTAssertEqual(p.records[0].data, ["name": "太郎", "hobby": "釣り"], "hidden・写真・computed・null は送らない")
        XCTAssertEqual(p.entityId, "profile")
    }

    func testPayloadStaysUnder16KB() throws {
        let (_, entity, _) = try fixture()
        let config = NearbyPassby.Config(componentId: "pb", entityId: "profile", label: "", fields: ["name", "hobby"], limit: 20)
        let big = String(repeating: "あ", count: 1500) // 約 4.5 KB / 件
        let records = (0..<30).map { Record(id: "r\($0)", data: ["name": .string("n\($0)"), "hobby": .string(big)]) }
        let p = NearbyPassby.makePayload(senderId: "s", name: "n", entity: entity, config: config, records: records,
                                         ledger: .init(), me: nil)
        XCTAssertLessThanOrEqual(try NearbyPassby.encode(p).count, NearbyPassby.maxBytes)
        XCTAssertEqual(p.records.count, 3)

        let small = (0..<30).map { Record(id: "r\($0)", data: ["name": .string("n\($0)")]) }
        let q = NearbyPassby.makePayload(senderId: "s", name: "n", entity: entity, config: config, records: small, ledger: .init(), me: nil)
        XCTAssertEqual(q.records.count, NearbyPassby.maxRecords)
    }

    func testSanitizeKeepsOnlyMyAllowedFields() throws {
        let (_, entity, config) = try fixture()
        let rows = (0..<25).map { NearbyPassby.Item(id: "r\($0)", data: ["name": "花子"]) }
        let payload = NearbyPassby.Payload(v: 1, senderId: "s2", name: "花子", entityId: "profile", records: [
            .init(id: "a", data: ["name": "花子", "secret": "答え", "photo": "data:x", "age": "二十", "unknown": 1]),
            .init(id: "b", data: ["secret": "only"]),
            .init(id: "", data: ["name": "id 無し"]),
        ] + rows)
        let items = NearbyPassby.sanitize(payload, into: entity, config: config)
        XCTAssertEqual(items.first, .init(id: "a", data: ["name": "花子"]), "未許可・型違いは捨てる")
        XCTAssertEqual(items.count, 18, "空の行・id 無しを除き、先頭 20 件まで")
        var other = payload
        other.entityId = "memo"
        XCTAssertTrue(NearbyPassby.sanitize(other, into: entity, config: config).isEmpty)
    }

    func testLedgerDedupesBySenderAndRecordId() {
        var ledger = NearbyPassby.Ledger()
        let first = ledger.plan([.init(id: "r1", data: ["name": "花子"])], from: "s2")
        XCTAssertEqual(first, [.create(key: "s2|r1", data: ["name": "花子"])])
        ledger.commit("s2|r1", ["name": "花子"])

        XCTAssertEqual(ledger.plan([.init(id: "r1", data: ["name": "花子"])], from: "s2"), [.skip(key: "s2|r1")], "同じ相手・同じ記録は 2 度追加しない")
        XCTAssertEqual(ledger.plan([.init(id: "r1", data: ["name": "はなこ"])], from: "s2"),
                       [.update(key: "s2|r1", old: ["name": "花子"], new: ["name": "はなこ"])], "変わったら更新")
        XCTAssertEqual(ledger.plan([.init(id: "r1", data: ["name": "花子"])], from: "s3"),
                       [.create(key: "s3|r1", data: ["name": "花子"])], "別の相手の同じ id は別物")

        let local = [Record(id: "tmp", data: ["name": "花子", "note": "自分で足したメモ"])]
        XCTAssertEqual(NearbyPassby.Ledger.match(["name": "花子"], in: local)?.id, "tmp", "仮 id が変わっても内容で探す")
        XCTAssertTrue(ledger.isReceived(["name": "花子", "hobby": "x"]))
        XCTAssertFalse(ledger.isReceived(["name": "太郎"]))
    }

    func testEncountersAndCooldown() throws {
        var ledger = NearbyPassby.Ledger()
        let t0 = Date(timeIntervalSince1970: 1_000_000)
        XCTAssertTrue(ledger.shouldExchange(with: "s2", now: t0))
        ledger.met("s2", name: "花子", at: t0, records: 1)
        XCTAssertFalse(ledger.shouldExchange(with: "s2", now: t0.addingTimeInterval(60)), "続けて交換しない")
        ledger.met("s3", name: "次郎", at: t0.addingTimeInterval(30), records: 0)
        ledger.met("s2", name: "花子さん", at: t0.addingTimeInterval(NearbyPassby.cooldown + 1), records: 1)
        XCTAssertEqual(ledger.encounters.map(\.id), ["s2", "s3"], "最後に会った順")
        let e = ledger.encounters[0]
        XCTAssertEqual(e.times, 2)
        XCTAssertEqual(e.records, 2)
        XCTAssertEqual(e.name, "花子さん")
        XCTAssertEqual(e.firstMet, t0)

        let data = try NearbyPassby.encode(ledger)
        XCTAssertEqual(try NearbyPassby.decode(NearbyPassby.Ledger.self, from: data), ledger)

        for i in 0..<(NearbyPassby.Ledger.maxEncounters + 5) { ledger.met("x\(i)", name: "x", at: t0, records: 0) }
        XCTAssertEqual(ledger.encounters.count, NearbyPassby.Ledger.maxEncounters)
    }

    func testConsentBeforeFirstAdvertising() {
        XCTAssertEqual(NearbyPassby.toggle(on: true, consentedVersion: nil), .askConsent)
        XCTAssertEqual(NearbyPassby.toggle(on: true, consentedVersion: 0), .askConsent)
        XCTAssertEqual(NearbyPassby.toggle(on: true, consentedVersion: NearbyPassby.consentVersion), .start)
        XCTAssertEqual(NearbyPassby.toggle(on: false, consentedVersion: nil), .stop)
        let text = NearbyPassby.consentText(fieldNames: ["名前", "趣味"])
        XCTAssertTrue(text.contains("「名前」「趣味」"))
        XCTAssertTrue(text.contains("裏に回すと止まります"))
    }

    func testAppTagAndFormatting() {
        XCTAssertEqual(NearbyPassby.appTag("app1"), NearbyPassby.appTag("app1"))
        XCTAssertNotEqual(NearbyPassby.appTag("app1"), NearbyPassby.appTag("app2"))
        XCTAssertFalse(NearbyPassby.appTag("app1").contains("app1"), "appId 自体は流さない")
        XCTAssertEqual(NearbyPassby.distanceText(0.84), "0.8 m")
        XCTAssertEqual(NearbyPassby.distanceText(12.4), "12 m")
        XCTAssertEqual(NearbyPassby.directionText(azimuth: 0), "正面")
        XCTAssertEqual(NearbyPassby.directionText(azimuth: .pi / 4), "右前")
        XCTAssertEqual(NearbyPassby.directionText(azimuth: -.pi / 2), "左")
    }

    func testHelloRoundTrip() throws {
        let hello = NearbyPassby.Hello(app: NearbyPassby.appTag("a"), senderId: "s1", name: "太郎")
        XCTAssertEqual(try NearbyPassby.decode(NearbyPassby.Hello.self, from: NearbyPassby.encode(hello)), hello)
    }
}
