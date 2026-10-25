import XCTest
import MetamonDSL
@testable import MetamonRenderer

/// #229: timePicker は未入力でも今の時刻を表示する → 見えている時刻を保存値にする(触らずに保存して出勤時刻が空 = 勤務時間「—」を防ぐ)。
final class TimePickerSeedTests: XCTestCase {
    private func shift() throws -> Entity {
        try JSONDecoder().decode(Entity.self, from: Data(#"""
        {"id":"shift","name":"シフト","fields":[
          {"id":"start","name":"出勤","type":"string","required":true},
          {"id":"end","name":"退勤","type":"string","defaultValue":"18:00"},
          {"id":"at","name":"日時","type":"datetime"}]}
        """#.utf8))
    }

    private var morning: Date { Calendar.current.date(from: DateComponents(year: 2026, month: 10, day: 25, hour: 9, minute: 5))! }

    @MainActor func testSeedsEmptyStringFieldWithShownTime() throws {
        let draft = FormDraft(entity: try shift())
        XCTAssertEqual(draft.missingRequired, ["出勤"])
        draft.seedTimeIfEmpty("start", now: morning)
        XCTAssertEqual(draft.values["start"], "09:05")
        XCTAssertEqual(draft.missingRequired, [])
    }

    @MainActor func testKeepsExistingValue() throws {
        let draft = FormDraft(entity: try shift())
        draft.seedTimeIfEmpty("end", now: morning)
        XCTAssertEqual(draft.values["end"], "18:00", "既定値・入力済みは上書きしない")
        draft.load(Record(id: "r1", data: ["start": "22:00"]))
        draft.seedTimeIfEmpty("start", now: morning)
        XCTAssertEqual(draft.values["start"], "22:00")
    }

    @MainActor func testSeedsDatetimeAsRFC3339() throws {
        let draft = FormDraft(entity: try shift())
        // datetime は初期化で既定値(今)が入るので、空にしてから seed を確かめる
        let initial = draft.values["at"]
        draft.seedTimeIfEmpty("at", now: morning)
        XCTAssertEqual(draft.values["at"], initial, "既定値の入った datetime は上書きしない")
        draft.values["at"] = .null
        draft.seedTimeIfEmpty("at", now: morning)
        let saved = try XCTUnwrap(draft.values["at"]?.stringValue)
        XCTAssertTrue(saved.contains("T"), "datetime は RFC3339")
        XCTAssertEqual(saved, FieldCoding.datetimeFormatter.string(from: morning), "datePicker の datetime と同じ形")
        // タイムゾーンに依らず同じ瞬間に戻る
        XCTAssertEqual(FieldCoding.date(from: .string(saved)), morning)
    }
}
