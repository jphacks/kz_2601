import SwiftUI
import XCTest
import MetamonDSL
import MetamonRenderer
@testable import Metamon

/// #1998: アシスティブアクセスで開いた生成アプリの大きなボタン(選び方・押した時の保存・見た目)。
@MainActor
final class AssistiveActionsTests: XCTestCase {
    private let today = FieldCoding.dateFormatter.date(from: "2026-11-12")!

    private func decode(_ json: String) throws -> AppDSL {
        try JSONDecoder().decode(AppDSL.self, from: Data(json.utf8))
    }

    /// 服薬のチェック(毎日リセット)+ 薬の追加フォーム + 今日飲んだ数。
    private func medication() throws -> AppDSL {
        try decode("""
        {"schemaVersion":1,"app":{"name":"服薬"},"initialScreen":"home",
         "entities":[{"id":"med","name":"薬","titleField":"name","fields":[
           {"id":"name","name":"名前","type":"string","required":true},
           {"id":"taken","name":"飲んだ日","type":"date"}]}],
         "screens":[
           {"id":"home","title":"服薬","components":[
             {"id":"count","type":"stat","label":"薬の数","entity":"med","aggregate":"count"},
             {"id":"list","type":"list","entity":"med","item":{"title":"name","toggle":"taken","resets":"daily"}}]},
           {"id":"add","title":"薬を追加","entity":"med","components":[]}]}
        """)
    }

    private var meds: [String: [Record]] {
        ["med": [
            Record(id: "m1", data: ["name": .string("朝の薬"), "taken": .string("2026-11-12")]),
            Record(id: "m2", data: ["name": .string("夜の薬"), "taken": .string("2026-11-11")]),
        ]]
    }

    /// 既定値だけで 1 件作れる記録(日時だけ必須)と、必須の数値がある記録。
    private func water() throws -> AppDSL {
        try decode("""
        {"schemaVersion":1,"app":{"name":"水分"},"initialScreen":"home",
         "entities":[
           {"id":"drink","name":"水を飲んだ","fields":[
             {"id":"at","name":"日時","type":"datetime","required":true},
             {"id":"memo","name":"メモ","type":"string"}]},
           {"id":"weight","name":"体重","fields":[
             {"id":"kg","name":"体重","type":"number","required":true}]}],
         "screens":[
           {"id":"home","title":"水分","components":[
             {"id":"times","type":"stat","label":"回数","entity":"drink","aggregate":"count"}]},
           {"id":"drink_form","title":"記録","entity":"drink","components":[]},
           {"id":"weight_form","title":"体重","entity":"weight","components":[]}]}
        """)
    }

    // MARK: - 選び方

    func testCheckRowsBecomeButtonsFirst() throws {
        let actions = AssistiveActions.pick(dsl: try medication(), records: meds, today: today)
        XCTAssertEqual(actions.map(\.title), ["朝の薬", "夜の薬", "薬の数"])
        XCTAssertEqual(actions.map(\.isOn), [true, false, false], "毎日リセット: 今日の日付なら済み")
        XCTAssertEqual(actions[0].kind, .check(entity: "med", recordId: "m1", field: "taken", daily: true))
        XCTAssertEqual(actions[2].value, "2 件")
    }

    func testQuickRecordNeedsNoInput() throws {
        let d = try water()
        let actions = AssistiveActions.pick(dsl: d, records: ["drink": []], today: today)
        XCTAssertEqual(actions.map(\.kind), [.quickRecord(entity: "drink"), .stat(componentId: "times")],
                       "必須の数値がある体重はワンタップで作れないので出さない")
        XCTAssertEqual(actions[0].title, "水を飲んだ")
        XCTAssertEqual(actions[1].value, "0 件")
    }

    func testStatsOnly() throws {
        let d = try decode("""
        {"schemaVersion":1,"app":{"name":"家計簿"},"initialScreen":"home",
         "entities":[{"id":"e","name":"支出","fields":[{"id":"amount","name":"金額","type":"number","required":true}]}],
         "screens":[{"id":"home","title":"家計簿","components":[
           {"id":"sum","type":"stat","label":"合計","entity":"e","aggregate":"sum","field":"amount"}]}]}
        """)
        let actions = AssistiveActions.pick(dsl: d, records: ["e": [Record(id: "a", data: ["amount": .number(800)])]], today: today)
        XCTAssertEqual(actions.map(\.kind), [.stat(componentId: "sum")])
        XCTAssertEqual(actions[0].title, "合計")
        XCTAssertTrue(AssistiveActions.hasCandidates(in: d))
    }

    func testAppWithNothingKeepsCurrentScreen() throws {
        let d = try decode("""
        {"schemaVersion":1,"app":{"name":"メモ"},"initialScreen":"home","entities":[],
         "screens":[{"id":"home","title":"メモ","components":[{"id":"t","type":"text","text":"こんにちは"}]}]}
        """)
        XCTAssertFalse(AssistiveActions.hasCandidates(in: d))
        XCTAssertEqual(AssistiveActions.pick(dsl: d, records: [:], today: today), [])
    }

    func testAtMostFour() throws {
        let rows = (1...6).map { Record(id: "m\($0)", data: ["name": .string("薬\($0)")]) }
        let actions = AssistiveActions.pick(dsl: try medication(), records: ["med": rows], today: today)
        XCTAssertEqual(actions.count, AssistiveActions.limit)
        XCTAssertEqual(actions.map(\.title), ["薬1", "薬2", "薬3", "薬4"])
    }

    func testSharedEntitiesAreNotPicked() throws {
        let d = try decode("""
        {"schemaVersion":1,"app":{"name":"掲示板"},"initialScreen":"home",
         "entities":[{"id":"post","name":"投稿","scope":"shared","fields":[
           {"id":"at","name":"日時","type":"datetime"},{"id":"done","name":"済み","type":"boolean"}]}],
         "screens":[
           {"id":"home","title":"掲示板","components":[{"id":"l","type":"list","entity":"post","item":{"title":"at","toggle":"done"}}]},
           {"id":"form","title":"投稿","entity":"post","components":[]}]}
        """)
        XCTAssertFalse(AssistiveActions.hasCandidates(in: d), "他の人に見える記録はワンタップで作らない・変えない")
    }

    func testEntitiesToLoad() throws {
        XCTAssertEqual(AssistiveActions.entities(in: try medication()), ["med"])
        XCTAssertEqual(AssistiveActions.entities(in: try water()), ["drink"])
    }

    // MARK: - 押した時の保存

    func testCheckWriteFlipsDailyCheck() throws {
        let d = try medication()
        let actions = AssistiveActions.pick(dsl: d, records: meds, today: today)
        let on = try XCTUnwrap(AssistiveActions.write(for: actions[1], in: d, records: meds, today: today))
        XCTAssertEqual(on["taken"], .string("2026-11-12"), "夜の薬に今日を付ける")
        let off = try XCTUnwrap(AssistiveActions.write(for: actions[0], in: d, records: meds, today: today))
        XCTAssertEqual(off["taken"], .null, "朝の薬は今日を外す")
    }

    func testBooleanCheckWrite() throws {
        let d = try decode("""
        {"schemaVersion":1,"app":{"name":"やること"},"initialScreen":"home",
         "entities":[{"id":"todo","name":"やること","fields":[
           {"id":"title","name":"件名","type":"string","required":true},{"id":"done","name":"済み","type":"boolean"}]}],
         "screens":[{"id":"home","title":"やること","components":[
           {"id":"l","type":"list","entity":"todo","item":{"title":"title","toggle":"done"}}]}]}
        """)
        let rows = ["todo": [Record(id: "t", data: ["title": .string("ゴミ出し"), "done": .bool(false)])]]
        let a = try XCTUnwrap(AssistiveActions.pick(dsl: d, records: rows, today: today).first)
        XCTAssertEqual(AssistiveActions.write(for: a, in: d, records: rows, today: today), ["done": .bool(true)])
    }

    func testQuickRecordWriteUsesDefaults() throws {
        let d = try water()
        let a = try XCTUnwrap(AssistiveActions.pick(dsl: d, records: [:], today: today).first)
        let data = try XCTUnwrap(AssistiveActions.write(for: a, in: d, records: [:], today: today))
        XCTAssertNotNil(data["at"]?.stringValue, "日時は今")
        XCTAssertNil(AssistiveActions.write(for: .init(kind: .stat(componentId: "times"), title: ""), in: d, records: [:]))
    }

    // MARK: - 見た目(日本語・英語・文字の大きさ最大)

    func testGridRendersInJapaneseEnglishAndLargestType() throws {
        let saved = LanguagePreference.shared.setting
        defer { LanguagePreference.shared.setting = saved }
        let d = try medication()
        var actions = AssistiveActions.pick(dsl: d, records: meds, today: today)
        actions.insert(.init(kind: .quickRecord(entity: "drink"), title: "水を飲んだ"), at: 2)
        let cases: [(String, AppLanguage, DynamicTypeSize)] = [
            ("assistive-ja", .ja, .large),
            ("assistive-en", .en, .large),
            ("assistive-ja-ax5", .ja, .accessibility5),
            ("assistive-en-ax5", .en, .accessibility5),
        ]
        for (name, language, type) in cases {
            LanguagePreference.shared.setting = language
            let view = AssistiveActionGrid(appName: "服薬", actions: actions, onTap: { _ in })
                .frame(width: 393)
                .fixedSize(horizontal: false, vertical: true)
                .background(Theme.Colors.canvas)
                .environment(\.dynamicTypeSize, type)
            let renderer = ImageRenderer(content: view)
            renderer.scale = 2
            let image = try XCTUnwrap(renderer.uiImage, name)
            XCTAssertEqual(image.size.width, 393, name)
            XCTAssertGreaterThan(image.size.height, 104 * 4, "\(name): 4 つのボタンが描かれる")
            try save(image, name: name)
        }
        for kind in [AssistiveRecordedNotice.Kind.recorded, .unchecked, .failed] {
            LanguagePreference.shared.setting = .en
            XCTAssertFalse(kind.text.isEmpty)
            XCTAssertNotNil(ImageRenderer(content: AssistiveRecordedNotice(kind: kind).environment(\.dynamicTypeSize, .accessibility5)).uiImage)
        }
        LanguagePreference.shared.setting = .en
        XCTAssertEqual(AssistiveRecordedNotice.Kind.recorded.text, "Recorded")
        LanguagePreference.shared.setting = .ja
        XCTAssertEqual(AssistiveRecordedNotice.Kind.recorded.text, "記録しました")
    }

    /// `ASSISTIVE_SHOTS_DIR` があれば PNG を書き出す(xcodebuild では `TEST_RUNNER_ASSISTIVE_SHOTS_DIR=<dir>`)。
    private func save(_ image: UIImage, name: String) throws {
        guard let dir = ProcessInfo.processInfo.environment["ASSISTIVE_SHOTS_DIR"], !dir.isEmpty else { return }
        try XCTUnwrap(image.pngData()).write(to: URL(fileURLWithPath: dir).appendingPathComponent("\(name).png"))
    }
}
