import XCTest
@testable import MetamonDSL

/// すれ違いの記録の付け足しの計画(NearbyPassby.Ledger の plan・shouldExchange・met と toggle)(下書き stock7-ios-012)。
final class NearbyPassbyEncounterTests: XCTestCase {
    private let t0 = Date(timeIntervalSince1970: 2_000_000)

    func testPlanCreatesUpdatesAndSkipsPerItem() {
        var ledger = NearbyPassby.Ledger()
        let a: [String: JSONValue] = ["name": .string("花子")]
        let b: [String: JSONValue] = ["name": .string("太郎")]
        ledger.commit(NearbyPassby.Ledger.key(sender: "s1", id: "r1"), a)
        ledger.commit(NearbyPassby.Ledger.key(sender: "s1", id: "r2"), b)

        let changed: [String: JSONValue] = ["name": .string("太郎さん")]
        let plans = ledger.plan([.init(id: "r1", data: a), .init(id: "r2", data: changed), .init(id: "r3", data: a)], from: "s1")
        XCTAssertEqual(plans, [
            .skip(key: "s1|r1"),
            .update(key: "s1|r2", old: b, new: changed),
            .create(key: "s1|r3", data: a),
        ])
    }

    func testPlanIsEmptyForNoItemsAndKeepsSendersApart() {
        var ledger = NearbyPassby.Ledger()
        XCTAssertEqual(ledger.plan([], from: "s1"), [])
        ledger.commit("s1|r1", ["name": .string("a")])
        XCTAssertEqual(ledger.plan([.init(id: "r1", data: ["name": .string("a")])], from: "s2"),
                       [.create(key: "s2|r1", data: ["name": .string("a")])])
    }

    func testPlanDoesNotMutateLedgerUntilCommit() {
        var ledger = NearbyPassby.Ledger()
        let item = NearbyPassby.Item(id: "r1", data: ["name": .string("a")])
        _ = ledger.plan([item], from: "s1")
        XCTAssertTrue(ledger.received.isEmpty)
        ledger.commit("s1|r1", item.data)
        XCTAssertEqual(ledger.plan([item], from: "s1"), [.skip(key: "s1|r1")])
    }

    func testShouldExchangeBoundaryAtCooldown() {
        var ledger = NearbyPassby.Ledger()
        XCTAssertTrue(ledger.shouldExchange(with: "s1", now: t0), "初対面")
        ledger.met("s1", name: "花子", at: t0, records: 1)
        XCTAssertFalse(ledger.shouldExchange(with: "s1", now: t0))
        XCTAssertFalse(ledger.shouldExchange(with: "s1", now: t0.addingTimeInterval(NearbyPassby.cooldown - 1)))
        XCTAssertTrue(ledger.shouldExchange(with: "s1", now: t0.addingTimeInterval(NearbyPassby.cooldown)), "ちょうど cooldown で交換する")
        XCTAssertTrue(ledger.shouldExchange(with: "s2", now: t0), "別の人は別")
    }

    func testMetCountsTimesOnlyAfterCooldownAndAccumulatesRecords() {
        var ledger = NearbyPassby.Ledger()
        ledger.met("s1", name: "花子", at: t0, records: 2)
        ledger.met("s1", name: "花子", at: t0.addingTimeInterval(NearbyPassby.cooldown - 1), records: 1)
        XCTAssertEqual(ledger.encounters.count, 1)
        XCTAssertEqual(ledger.encounters[0].times, 1, "間が空いていない時は回数を増やさない")
        XCTAssertEqual(ledger.encounters[0].records, 3)
        XCTAssertEqual(ledger.encounters[0].firstMet, t0)
        XCTAssertEqual(ledger.encounters[0].lastMet, t0.addingTimeInterval(NearbyPassby.cooldown - 1))

        ledger.met("s1", name: "花子", at: t0.addingTimeInterval(NearbyPassby.cooldown * 3), records: 0)
        XCTAssertEqual(ledger.encounters[0].times, 2)
    }

    func testMetKeepsNewestFirstAndDropsOldestOverCap() {
        var ledger = NearbyPassby.Ledger()
        for i in 0..<NearbyPassby.Ledger.maxEncounters { ledger.met("p\(i)", name: "x", at: t0, records: 0) }
        XCTAssertEqual(ledger.encounters.count, NearbyPassby.Ledger.maxEncounters)
        XCTAssertEqual(ledger.encounters.first?.id, "p\(NearbyPassby.Ledger.maxEncounters - 1)")
        ledger.met("new", name: "x", at: t0, records: 0)
        XCTAssertEqual(ledger.encounters.count, NearbyPassby.Ledger.maxEncounters)
        XCTAssertEqual(ledger.encounters.first?.id, "new")
        XCTAssertFalse(ledger.encounters.contains { $0.id == "p0" }, "いちばん古い人が消える")
    }

    func testCommitCapsReceivedAtMax() {
        var ledger = NearbyPassby.Ledger()
        for i in 0...NearbyPassby.Ledger.maxReceived { ledger.commit(String(format: "s|%05d", i), ["n": .number(Double(i))]) }
        XCTAssertEqual(ledger.received.count, NearbyPassby.Ledger.maxReceived)
    }

    func testToggleDependsOnConsentVersion() {
        XCTAssertEqual(NearbyPassby.toggle(on: false, consentedVersion: nil), .stop)
        XCTAssertEqual(NearbyPassby.toggle(on: false, consentedVersion: NearbyPassby.consentVersion), .stop)
        XCTAssertEqual(NearbyPassby.toggle(on: true, consentedVersion: nil), .askConsent)
        XCTAssertEqual(NearbyPassby.toggle(on: true, consentedVersion: 0), .askConsent)
        XCTAssertEqual(NearbyPassby.toggle(on: true, consentedVersion: NearbyPassby.consentVersion - 1), .askConsent, "同意文の版が古ければ取り直す")
        XCTAssertEqual(NearbyPassby.toggle(on: true, consentedVersion: NearbyPassby.consentVersion), .start)
        XCTAssertEqual(NearbyPassby.toggle(on: true, consentedVersion: NearbyPassby.consentVersion + 1), .start)
    }
}
