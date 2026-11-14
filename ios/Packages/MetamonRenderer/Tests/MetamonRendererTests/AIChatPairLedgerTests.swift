import XCTest
import MetamonDSL
@testable import MetamonRenderer

/// roleField の無い aiChat の 1 往復を対で保存する中身(iOS / Mac 共通・#1326)。
/// 役は並び順で読むので、片方だけを残さない(作れない・断られた時)。
@MainActor
final class AIChatPairLedgerTests: XCTestCase {
    /// 作った順に並ぶローカル DB の代わり。`failOn` の本文は作れない。
    private final class Store {
        var rows: [Record] = []
        var failOn: Set<String> = []
        private var next = 0

        func create(_ data: [String: JSONValue]) -> String? {
            if let text = data["text"]?.stringValue, failOn.contains(text) { return nil }
            next += 1
            let id = "local-\(next)"
            rows.append(Record(id: id, data: data, createdAt: String(format: "2026-01-01T00:00:%02dZ", next)))
            return id
        }

        func remove(_ id: String) { rows.removeAll { $0.id == id } }

        var turns: [AIChat.Turn] { AIChat.turns(rows, textField: "text", roleField: nil) }
    }

    private func save(_ ledger: AIChatPairLedger, _ store: Store, _ user: String, _ ai: String,
                      appId: String = "a", isCurrent: () -> Bool = { true }) async -> (user: String, assistant: String)? {
        await ledger.save(appId: appId, entity: "turn", user: ["text": .string(user)], assistant: ["text": .string(ai)],
                          isCurrent: isCurrent, create: { store.create($0) }, remove: { store.remove($0) })
    }

    func testSavesUserThenAssistantAndRemembersBothWays() async {
        let ledger = AIChatPairLedger(), store = Store()
        let ids = await save(ledger, store, "眠れない", "いつからですか")
        XCTAssertEqual(store.turns.map(\.text), ["眠れない", "いつからですか"], "自分 → AI の順に作る")
        XCTAssertEqual(store.turns.map(\.role), [.user, .assistant])
        XCTAssertEqual(ids?.user, "local-1")
        XCTAssertEqual(ids?.assistant, "local-2")
        XCTAssertEqual(ledger.partners["local-1"]?.partner, "local-2")
        XCTAssertEqual(ledger.partners["local-2"]?.partner, "local-1", "どちらが断られても相方を引ける")
    }

    func testReplyNotSavedRemovesUserTurn() async {
        let ledger = AIChatPairLedger(), store = Store()
        _ = await save(ledger, store, "1", "2")
        store.failOn = ["4"]
        let ids = await save(ledger, store, "3", "4")
        XCTAssertNil(ids)
        XCTAssertEqual(store.turns.map(\.text), ["1", "2"], "返事を保存できなければ自分の発言(3)も残さない")
        XCTAssertEqual(ledger.partners.count, 2)
    }

    func testUserNotSavedCreatesNothing() async {
        let ledger = AIChatPairLedger(), store = Store()
        store.failOn = ["1"]
        let ids = await save(ledger, store, "1", "2")
        XCTAssertNil(ids)
        XCTAssertTrue(store.rows.isEmpty, "自分の発言を作れなければ返事も作らない")
        XCTAssertTrue(ledger.partners.isEmpty)
    }

    func testSwitchedAppDuringSaveRemovesUserTurn() async {
        let ledger = AIChatPairLedger(), store = Store()
        let ids = await save(ledger, store, "1", "2", isCurrent: { false })
        XCTAssertNil(ids)
        XCTAssertTrue(store.rows.isEmpty, "別のアプリへ切り替わったら返事を作らず、自分の発言も消す")
    }

    /// 返事が後でサーバーに断られた(キューがローカル行を消した): 相方(自分の発言)を引いて消すと交互が崩れない。
    func testRefusedReplyGivesUserTurnToRemove() async throws {
        let ledger = AIChatPairLedger(), store = Store()
        _ = await save(ledger, store, "1", "2")
        let saved = await save(ledger, store, "3", "4")
        let second = try XCTUnwrap(saved)
        store.remove(second.assistant)
        let partner = ledger.takePartner(appId: "a", entity: "turn", recordId: second.assistant)
        XCTAssertEqual(partner, second.user)
        store.remove(try XCTUnwrap(partner))
        _ = await save(ledger, store, "5", "6")
        XCTAssertEqual(store.turns.map(\.text), ["1", "2", "5", "6"])
        XCTAssertEqual(store.turns.map(\.role), [.user, .assistant, .user, .assistant], "断られた後も自分 → AI の交互のまま")
        XCTAssertEqual(ledger.partners.count, 4, "消した対は忘れる")
        XCTAssertNil(ledger.takePartner(appId: "a", entity: "turn", recordId: second.user), "同じ対を二度消さない")
    }

    func testRefusalOfOtherRecordsIsIgnored() async throws {
        let ledger = AIChatPairLedger(), store = Store()
        let saved = await save(ledger, store, "1", "2")
        let ids = try XCTUnwrap(saved)
        XCTAssertNil(ledger.takePartner(appId: "a", entity: "turn", recordId: "local-other"))
        XCTAssertNil(ledger.takePartner(appId: "b", entity: "turn", recordId: ids.user), "別のアプリの断りでは消さない")
        XCTAssertNil(ledger.takePartner(appId: "a", entity: "note", recordId: ids.user), "別の entity の断りでは消さない")
        XCTAssertEqual(ledger.partners.count, 2)
    }
}
