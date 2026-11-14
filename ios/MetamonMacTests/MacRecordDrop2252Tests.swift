import XCTest
import MetamonDSL
@testable import Metamon

/// 記録の行をサイドバーの別のアプリへドラッグ(#2252): 落とし先の entity を決める(1 つ・複数・合うものが無い)・預かり
@MainActor
final class MacRecordDrop2252Tests: XCTestCase {
    private func dsl(_ json: String) throws -> AppDSL {
        try JSONDecoder().decode(AppDSL.self, from: Data(json.utf8))
    }

    private func app(screens: String, entities: String) throws -> AppDSL {
        try dsl("""
        {"schemaVersion":1,"app":{"name":"テスト"},"initialScreen":"home","entities":[\(entities)],
         "screens":[{"id":"home","title":"ホーム","components":[]},\(screens)]}
        """)
    }

    private let payload = RecordClipboard.Payload(
        appId: "kakeibo", appName: "家計簿", entityId: "expense", entityName: "支出",
        fields: [.init(id: "amount", name: "金額", type: "currency", value: .number(1200)),
                 .init(id: "memo", name: "メモ", type: "string", value: .string("昼ごはん"))])

    func testSingleMatchingForm() throws {
        let d = try app(screens: #"{"id":"note_form","title":"メモを書く","entity":"notes","components":[]}"#,
                        entities: #"{"id":"notes","name":"メモ","fields":[{"id":"body","name":"メモ","type":"text"}]}"#)
        let targets = MacRecordDrop.targets(payload, dsl: d, appId: "other")
        XCTAssertEqual(targets, [.init(entity: "notes", name: "メモ", screen: "note_form", matched: 1)])
    }

    func testMultipleFormsAreOfferedInScreenOrder() throws {
        let d = try app(screens: """
            {"id":"a_form","title":"A","entity":"a","components":[]},
            {"id":"a_form2","title":"A2","entity":"a","components":[]},
            {"id":"b_form","title":"B","entity":"b","components":[]}
            """,
            entities: """
            {"id":"a","name":"出費","fields":[{"id":"price","name":"金額","type":"integer"}]},
            {"id":"b","name":"日記","fields":[{"id":"memo","name":"メモ","type":"text"}]}
            """)
        let targets = MacRecordDrop.targets(payload, dsl: d, appId: "other")
        XCTAssertEqual(targets.map(\.entity), ["a", "b"], "entity ごとに 1 つ・画面の順")
        XCTAssertEqual(targets.first?.screen, "a_form", "同じ entity のフォームは先の画面")
    }

    func testNoMatchingForm() throws {
        let d = try app(screens: #"{"id":"w_form","title":"体重","entity":"w","components":[]}"#,
                        entities: #"{"id":"w","name":"体重","fields":[{"id":"kg","name":"体重","type":"number"}]}"#)
        XCTAssertTrue(MacRecordDrop.targets(payload, dsl: d, appId: "other").isEmpty)
    }

    func testScreensWithoutEntityAreIgnored() throws {
        let d = try app(screens: #"{"id":"list","title":"一覧","components":[]}"#,
                        entities: #"{"id":"notes","name":"メモ","fields":[{"id":"memo","name":"メモ","type":"text"}]}"#)
        XCTAssertTrue(MacRecordDrop.targets(payload, dsl: d, appId: "other").isEmpty, "フォームが無ければ候補にしない")
    }

    func testInboxGivesPayloadOnceToTheRightApp() {
        let inbox = MacRecordDropInbox()
        let now = Date()
        inbox.put(payload, appId: "b", now: now)
        XCTAssertEqual(inbox.serial, 1)
        XCTAssertNil(inbox.take(appId: "a", now: now), "別のアプリは受け取らない")
        XCTAssertEqual(inbox.take(appId: "b", now: now), payload)
        XCTAssertNil(inbox.take(appId: "b", now: now), "1 回だけ")
        inbox.put(payload, appId: "b", now: now)
        XCTAssertNil(inbox.take(appId: "b", now: now.addingTimeInterval(MacRecordDropInbox.ttl + 1)), "古いものは捨てる")
    }
}
