import XCTest
@testable import MetamonDSL

/// `nearbyShare`(#36)の送受信データ・取り込み判定。MultipeerConnectivity を通さずに検証する。
final class NearbyShareTests: XCTestCase {
    private func attendance() throws -> (AppDSL, Entity) {
        let dsl = try ExamplesFixture.load("attendance_nearby")
        return (dsl, try XCTUnwrap(dsl.entity("attendance")))
    }

    func testExampleUsesNearbyShareOnFormAndList() throws {
        let dsl = try ExamplesFixture.load("attendance_nearby")
        let shares = dsl.screens.flatMap { s in s.components.filter { $0.type == .nearbyShare }.map { (s, $0) } }
        XCTAssertEqual(shares.count, 2)
        XCTAssertTrue(shares.contains { $0.0.entity == $0.1.entity }, "フォーム画面の nearbyShare")
        XCTAssertTrue(shares.contains { $0.0.entity == nil && $0.1.entity == "members" }, "一覧画面の nearbyShare")
        XCTAssertTrue(shares.allSatisfy { $0.1.label?.isEmpty == false })
    }

    func testPayloadRoundTripDropsUnknownAndNull() throws {
        let (_, entity) = try attendance()
        let payload = NearbyShare.makePayload(sender: "花子", appId: "app1", appName: "出席カード", entity: entity, records: [
            ["name": .string("花子"), "status": .string("出席"), "secret": .string("x"), "date": .null],
            ["secret": .string("only unknown")],
        ])
        XCTAssertEqual(payload.header.count, 1)
        XCTAssertEqual(payload.records, [["name": .string("花子"), "status": .string("出席")]])
        XCTAssertEqual(Set(payload.header.fields.map(\.id)), ["name", "status", "date"])

        let data = try NearbyShare.encode(payload)
        XCTAssertEqual(try NearbyShare.decode(NearbyShare.Payload.self, from: data), payload)
    }

    func testDecodeIgnoresUnknownTopLevelKeys() throws {
        let json = """
        {"header":{"v":1,"sender":"太郎","appId":"a","appName":"x","entityId":"attendance","entityName":"出席",
          "fields":[{"id":"name","type":"string"}],"count":1,"future":true},
         "records":[{"name":"太郎"}],"extra":[1,2]}
        """
        let p = try NearbyShare.decode(NearbyShare.Payload.self, from: Data(json.utf8))
        XCTAssertEqual(p.header.sender, "太郎")
        XCTAssertEqual(p.records.first?["name"], .string("太郎"))
    }

    func testDecideSameAppAcceptsEvenIfFieldsDiffer() throws {
        let (dsl, entity) = try attendance()
        var h = NearbyShare.Header(sender: "s", appId: "app1", appName: "x", entity: entity, count: 1)
        h.fields = [.init(id: "name", type: "string")]
        XCTAssertEqual(NearbyShare.decide(h, dsl: dsl, appId: "app1"), .accept(entityId: "attendance", sameApp: true))
    }

    func testDecideOtherAppNeedsSameFields() throws {
        let (dsl, entity) = try attendance()
        let h = NearbyShare.Header(sender: "s", appId: "other", appName: "x", entity: entity, count: 1)
        XCTAssertEqual(NearbyShare.decide(h, dsl: dsl, appId: "app1"), .accept(entityId: "attendance", sameApp: false))

        var changed = h
        changed.fields.append(.init(id: "room", type: "string"))
        guard case .reject(let reason) = NearbyShare.decide(changed, dsl: dsl, appId: "app1") else { return XCTFail("reject 期待") }
        XCTAssertTrue(reason.contains("項目が違う"), reason)

        var typeChanged = h
        typeChanged.fields = h.fields.map { $0.id == "date" ? .init(id: "date", type: "string") : $0 }
        XCTAssertNotEqual(NearbyShare.decide(typeChanged, dsl: dsl, appId: "app1"), .accept(entityId: "attendance", sameApp: false))
    }

    func testDecideMissingEntityRejects() throws {
        let (_, entity) = try attendance()
        let kakeibo = try ExamplesFixture.load("kakeibo")
        let h = NearbyShare.Header(sender: "s", appId: "app1", appName: "x", entity: entity, count: 3)
        guard case .reject(let reason) = NearbyShare.decide(h, dsl: kakeibo, appId: "app1") else { return XCTFail("reject 期待") }
        XCTAssertTrue(reason.contains("「出席」がない"), reason)
    }

    func testSanitizeDropsUnknownWrongTypeAndCrossAppRef() throws {
        let warikan = try ExamplesFixture.load("warikan")
        let entity = try XCTUnwrap(warikan.entities.first { $0.storedFields.contains { $0.type == .ref } })
        let ref = try XCTUnwrap(entity.storedFields.first { $0.type == .ref })
        let num = entity.storedFields.first { $0.type.isNumeric }
        var row: [String: JSONValue] = [ref.id: .string("r1"), "unknown": .string("x")]
        if let num { row[num.id] = .string("not a number") }

        XCTAssertEqual(NearbyShare.sanitize([row], into: entity, sameApp: false), [], "ref は別アプリだと捨てる・型違いも捨てる")
        XCTAssertEqual(NearbyShare.sanitize([row], into: entity, sameApp: true), [[ref.id: .string("r1")]])
    }

    func testRecordsToSendNewestFirstAndCapped() {
        let records = (0..<60).map { i in
            Record(id: "\(i)", data: ["n": .number(Double(i))], createdAt: String(format: "2026-10-24T10:%02d:00Z", i))
        }
        let out = NearbyShare.recordsToSend(from: records)
        XCTAssertEqual(out.count, NearbyShare.maxSendRecords)
        XCTAssertEqual(out.first?["n"], .number(59))
    }

    func testReceiveCap() throws {
        let (_, entity) = try attendance()
        let many = Array(repeating: ["name": JSONValue.string("a")], count: NearbyShare.maxReceiveRecords + 10)
        XCTAssertEqual(NearbyShare.sanitize(many, into: entity, sameApp: true).count, NearbyShare.maxReceiveRecords)
    }
}
