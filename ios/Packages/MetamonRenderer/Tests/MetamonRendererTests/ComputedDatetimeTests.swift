import XCTest
import MetamonDSL
@testable import MetamonRenderer

/// #747: computed の datetime(addMinutes で到着 − 45 分 = 出発)は保存値・一覧・フォーム・本文の {{}} で時刻まで出す。
@MainActor
final class ComputedDatetimeTests: XCTestCase {
    private let json = #"""
    {"schemaVersion":1,"app":{"name":"乗換"},"initialScreen":"home",
     "entities":[{"id":"trips","name":"予定","fields":[
       {"id":"title","name":"用件","type":"string"},
       {"id":"arrive_at","name":"到着","type":"datetime"},
       {"id":"depart_at","name":"出発","type":"datetime","computed":{"op":"addMinutes","args":[{"ref":"arrive_at"},-45]}}]}],
     "screens":[{"id":"home","title":"予定","components":[{"type":"list","entity":"trips"}]}]}
    """#

    private var saved: (tz: TimeZone, lang: AppLanguage, clock: Date?)?

    override func setUp() async throws {
        saved = (NSTimeZone.default, LanguagePreference.shared.setting, DSLClock.fixed)
        NSTimeZone.default = TimeZone(identifier: "Asia/Tokyo")!
        LanguagePreference.shared.setting = .ja
        DSLClock.fixed = ISO8601DateFormatter().date(from: "2026-11-12T12:00:00+09:00")
    }

    override func tearDown() async throws {
        guard let saved else { return }
        NSTimeZone.default = saved.tz
        LanguagePreference.shared.setting = saved.lang
        DSLClock.fixed = saved.clock
    }

    func testAddMinutesDatetimeKeepsTimeEverywhere() throws {
        let dsl = try JSONDecoder().decode(AppDSL.self, from: Data(json.utf8))
        let entity = try XCTUnwrap(dsl.entity("trips"))
        let depart = try XCTUnwrap(entity.field("depart_at"))
        XCTAssertTrue(depart.isComputed, "computed の datetime を読める")
        XCTAssertEqual(depart.type, .datetime)

        let record = Record(id: "t1", data: ["title": "面接", "arrive_at": "2026-11-14T09:00:00+09:00"])
        let rt = ConformanceRuntime(dsl: dsl, store: ["trips": [record]], currentUserId: "me")

        // 保存値(表示用レコード): RFC3339 で同じ時点(08:15 JST)
        let view = try XCTUnwrap(rt.viewRecords(for: "trips").first)
        let stored = try XCTUnwrap(FieldCoding.date(from: view["depart_at"]))
        XCTAssertEqual(stored, ISO8601DateFormatter().date(from: "2026-11-14T08:15:00+09:00"))

        // 一覧・表・行(rt.display)
        XCTAssertEqual(rt.display(view["depart_at"], field: depart), "11/14(土) 8:15")

        // フォーム(読み取り専用行・適合テストの木も同じ値)
        let form = FormDraft(entity: entity, record: record)
        XCTAssertEqual(ComputedFieldRow.resolve(field: depart, form: form, rt: rt).text, "11/14(土) 8:15")

        // 本文の {{depart_at}}(以前は ExprValue.text で「2026-11-14」と時刻が消えた)
        XCTAssertEqual(TextContent.resolve("出発 {{depart_at}}", form: form) { rt.evaluator() }, "出発 11/14(土) 8:15")

        // QR は保存値の形(RFC3339)で時刻を落とさない
        let qr = try JSONDecoder().decode(Component.self, from: Data(#"{"type":"qrCode","field":"depart_at"}"#.utf8))
        let qrText = QRCodeContent.resolve(qr, form: form) { rt.evaluator() }
        XCTAssertEqual(FieldCoding.date(from: .string(qrText)), stored, qrText)
    }

    /// #747 より前の端末: 知らない op は評価できない(nil)だけで落ちない → 「—」。
    func testUnknownOpOnComputedDatetimeShowsUnavailable() throws {
        let old = json.replacingOccurrences(of: "addMinutes", with: "futureOp")
        let dsl = try JSONDecoder().decode(AppDSL.self, from: Data(old.utf8))
        let depart = try XCTUnwrap(dsl.entity("trips")?.field("depart_at"))
        let rt = ConformanceRuntime(dsl: dsl, store: ["trips": [Record(id: "t1", data: ["arrive_at": "2026-11-14T09:00:00+09:00"])]], currentUserId: nil)
        let view = try XCTUnwrap(rt.viewRecords(for: "trips").first)
        XCTAssertNil(view.data["depart_at"])
        XCTAssertEqual(rt.display(view["depart_at"], field: depart), FieldCoding.unavailable)
    }
}
