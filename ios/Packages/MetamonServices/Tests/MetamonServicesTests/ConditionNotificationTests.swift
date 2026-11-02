import XCTest
import MetamonDSL
@testable import MetamonServices

/// #265: 条件で知らせる通知。偽 → 真で 1 回、真のままは出さない、偽に戻ってからまた真で出す。
final class ConditionNotificationTests: XCTestCase {
    /// 品目(在庫 = 入荷 − 出荷の computed)と入出荷。「在庫が 5 を下回った品目が 1 つ以上」で知らせる。
    static let inventoryJSON = """
    {"schemaVersion":1,"app":{"name":"在庫","voiceEntry":"floating","notifications":[
      {"id":"low","title":"在庫が少なくなりました","body":"5 個を下回った商品があります","repeat":"once",
       "when":{"op":"gt","args":[{"agg":"count","entity":"items","filter":{"field":"stock","op":"lt","value":5}},0]}},
      {"id":"daily","title":"記録","body":"","repeat":"daily","time":"20:00"}]},
     "initialScreen":"home",
     "entities":[
      {"id":"items","name":"商品","fields":[{"id":"name","name":"名前","type":"string"},
        {"id":"stock","name":"在庫","type":"integer","computed":{"agg":"sum","entity":"moves","field":"qty","filter":{"field":"item","op":"eq","value":{"ref":"id"}}}}]},
      {"id":"moves","name":"入出荷","fields":[{"id":"item","name":"商品","type":"ref","refEntity":"items"},{"id":"qty","name":"数","type":"integer"}]}],
     "screens":[{"id":"home","title":"在庫","components":[{"type":"list","entity":"items"}]}]}
    """

    private func dsl() throws -> AppDSL {
        try JSONDecoder().decode(AppDSL.self, from: Data(Self.inventoryJSON.utf8))
    }

    func testTransitionFiresOncePerFalseToTrue() {
        // 初めての評価は覚えるだけ(足した時点で満たしていても鳴らさない)
        XCTAssertEqual(ConditionNotifications.transition(previous: [:], current: ["low": true]).fire, [])
        // false → true は出す
        XCTAssertEqual(ConditionNotifications.transition(previous: ["low": false], current: ["low": true]).fire, ["low"])
        // true → true は出さない
        XCTAssertEqual(ConditionNotifications.transition(previous: ["low": true], current: ["low": true]).fire, [])
        // true → false は出さず、false を覚える
        let reset = ConditionNotifications.transition(previous: ["low": true], current: ["low": false])
        XCTAssertEqual(reset.fire, [])
        XCTAssertEqual(reset.next, ["low": false])
        // DSL から消えた通知は忘れる
        XCTAssertEqual(ConditionNotifications.transition(previous: ["gone": false], current: ["low": false]).next, ["low": false])
    }

    func testStorePersistsAndFiresAgainAfterReset() async {
        let suite = "condition-test-\(UUID().uuidString)"
        let defaults = UserDefaults(suiteName: suite)!
        defer { defaults.removePersistentDomain(forName: suite) }
        let store = ConditionNotificationStore(defaults: defaults)
        var fired: [[String]] = []
        for now in [false, true, true, false, true] {
            fired.append(await store.update(appId: "a", current: ["low": now]))
        }
        XCTAssertEqual(fired, [[], ["low"], [], [], ["low"]])
        // アプリを開き直しても(新しい store)前回の真を覚えていて、真のままなら出さない
        let reopened = ConditionNotificationStore(defaults: defaults)
        let again = await reopened.update(appId: "a", current: ["low": true])
        XCTAssertEqual(again, [])
        await reopened.removeApp("a")
        let state = await reopened.state(appId: "a")
        XCTAssertTrue(state.isEmpty)
    }

    /// 完了の条件: 出荷を記録して在庫が閾値を下回ると、その保存の後の評価で出る。
    func testShipmentBelowThresholdFires() async throws {
        let dsl = try dsl()
        let spec = try XCTUnwrap(dsl.app.notifications.first { $0.id == "low" })
        XCTAssertTrue(spec.isConditional)
        XCTAssertFalse(spec.isRecordBased)
        let items = [Record(id: "pen", data: ["name": .string("ペン")])]
        var moves = [Record(id: "in1", data: ["item": .string("pen"), "qty": .number(10)])]
        let suite = "condition-test-\(UUID().uuidString)"
        let defaults = UserDefaults(suiteName: suite)!
        defer { defaults.removePersistentDomain(forName: suite) }
        let store = ConditionNotificationStore(defaults: defaults)

        func step() async -> [String] {
            let current = ConditionNotifications.evaluate(dsl.app.notifications, dsl: dsl, records: ["items": items, "moves": moves])
            XCTAssertNil(current["daily"], "時刻の通知は評価しない")
            return await store.update(appId: "inv", current: current)
        }
        let baseline = await step() // 在庫 10
        XCTAssertEqual(baseline, [])
        moves.append(Record(id: "out1", data: ["item": .string("pen"), "qty": .number(-3)])) // 7
        let above = await step()
        XCTAssertEqual(above, [])
        moves.append(Record(id: "out2", data: ["item": .string("pen"), "qty": .number(-4)])) // 3 < 5
        let below = await step()
        XCTAssertEqual(below, ["low"])
        moves.append(Record(id: "out3", data: ["item": .string("pen"), "qty": .number(-1)])) // 2: 下回ったまま
        let still = await step()
        XCTAssertEqual(still, [])
        moves.append(Record(id: "in2", data: ["item": .string("pen"), "qty": .number(10)])) // 12: 戻った
        let restocked = await step()
        XCTAssertEqual(restocked, [])
        moves.append(Record(id: "out4", data: ["item": .string("pen"), "qty": .number(-8)])) // 4: また下回った
        let again = await step()
        XCTAssertEqual(again, ["low"])
    }

    /// 条件の通知は時刻の予約に入れない(古い版のように 9:00 の 1 回きりにしない)。
    func testPlannerSkipsConditional() throws {
        let dsl = try dsl()
        let planned = NotificationPlanner.plan(appId: "inv", specs: dsl.app.notifications, records: [:])
        XCTAssertEqual(planned.map(\.identifier), ["dsl.inv.daily"])
        let spec = try XCTUnwrap(dsl.app.notifications.first { $0.id == "low" })
        XCTAssertNil(NotificationPlanner.action(appId: "inv", spec: spec))
        XCTAssertEqual(ConditionNotifications.identifier(appId: "inv", specId: "low"), "when.inv.low")
    }

    /// 読めない when(未知の形)は通知ごと落とす(時刻の通知として誤って鳴らさない)。
    func testUnreadableWhenDropsTheNotification() throws {
        let json = Self.inventoryJSON.replacingOccurrences(
            of: #""when":{"op":"gt""#, with: #""when":{"bogus":1,"x":"gt""#)
        let dsl = try JSONDecoder().decode(AppDSL.self, from: Data(json.utf8))
        XCTAssertEqual(dsl.app.notifications.map(\.id), ["daily"])
    }
}
