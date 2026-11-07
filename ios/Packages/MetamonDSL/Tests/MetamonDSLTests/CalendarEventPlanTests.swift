import Foundation
import XCTest
@testable import MetamonDSL

/// addToCalendar: レコード → 「新規イベント」画面の中身。
final class CalendarEventPlanTests: XCTestCase {
    private func entity() throws -> Entity {
        try JSONDecoder().decode(Entity.self, from: Data(#"""
        {"id":"event","name":"イベント","titleField":"name","fields":[
          {"id":"name","name":"名前","type":"string"},
          {"id":"day","name":"日","type":"date"},
          {"id":"start_at","name":"開始","type":"datetime"},
          {"id":"end_at","name":"終了","type":"datetime"},
          {"id":"place","name":"場所","type":"string"},
          {"id":"memo","name":"メモ","type":"text"},
          {"id":"round","name":"回","type":"integer"}]}
        """#.utf8))
    }

    private func draft(_ spec: CalendarEventSpec, _ data: [String: JSONValue]) throws -> CalendarEventDraft {
        try CalendarEventPlan.build(spec, entity: try entity(), data: data).get()
    }

    private let start = "2026-11-14T10:00:00+09:00"
    private var startDate: Date { FieldCoding.datetimeFormatter.date(from: start)! }

    func testTimedEventDefaultsToSixtyMinutes() throws {
        let d = try draft(CalendarEventSpec(startField: "start_at"), ["name": .string(" JPHACKS 本戦 "), "start_at": .string(start)])
        XCTAssertEqual(d.title, "JPHACKS 本戦")
        XCTAssertFalse(d.isAllDay)
        XCTAssertEqual(d.start, startDate)
        XCTAssertEqual(d.end.timeIntervalSince(d.start), 3600)
        XCTAssertNil(d.alertOffset)
        XCTAssertNil(d.location)
    }

    func testEndFieldLocationNotesAndAlert() throws {
        let spec = CalendarEventSpec(startField: "start_at", endField: "end_at", durationMinutes: 30, titleField: "name",
                                     locationField: "place", notesField: "memo", alertMinutes: 1440)
        let d = try draft(spec, ["name": .string("発表"), "start_at": .string(start), "end_at": .string("2026-11-14T12:30:00+09:00"),
                                 "place": .string("金沢"), "memo": .string("スライド持参")])
        XCTAssertEqual(d.end.timeIntervalSince(d.start), 2.5 * 3600)
        XCTAssertEqual(d.location, "金沢")
        XCTAssertEqual(d.notes, "スライド持参")
        XCTAssertEqual(d.alertOffset, -86400)
    }

    func testEmptyOrEarlierEndFallsBackToDuration() throws {
        let spec = CalendarEventSpec(startField: "start_at", endField: "end_at", durationMinutes: 90)
        for end: JSONValue in [.null, .string(""), .string("2026-11-14T09:00:00+09:00")] {
            let d = try draft(spec, ["start_at": .string(start), "end_at": end])
            XCTAssertEqual(d.end.timeIntervalSince(d.start), 5400, "\(end)")
        }
    }

    func testDateFieldIsAllDay() throws {
        let d = try draft(CalendarEventSpec(startField: "day", alertMinutes: 0), ["day": .string("2026-11-14")])
        XCTAssertTrue(d.isAllDay)
        XCTAssertEqual(d.start, FieldCoding.dateFormatter.date(from: "2026-11-14"))
        XCTAssertEqual(d.end, d.start)
        XCTAssertEqual(d.alertOffset, 0)
    }

    func testTitleFallsBackToEntityName() throws {
        XCTAssertEqual(try draft(CalendarEventSpec(startField: "start_at"), ["start_at": .string(start)]).title, "イベント")
        XCTAssertEqual(try draft(CalendarEventSpec(startField: "start_at", titleField: "round"),
                                 ["start_at": .string(start), "round": .number(3)]).title, "3")
    }

    func testMissingStartFailsWithFieldName() throws {
        let r = CalendarEventPlan.build(CalendarEventSpec(startField: "start_at"), entity: try entity(), data: ["start_at": .string("あした")])
        XCTAssertEqual(r, .failure(.noStart("開始")))
    }

    func testActionDecodes() throws {
        let raw = #"{"type":"addToCalendar","entity":"event","calendarEvent":{"startField":"start_at","alertMinutes":0,"locationField":"place"}}"#
        let a = try JSONDecoder().decode(Action.self, from: Data(raw.utf8))
        XCTAssertEqual(a.type, .addToCalendar)
        XCTAssertEqual(a.entity, "event")
        XCTAssertEqual(a.calendarEvent, CalendarEventSpec(startField: "start_at", locationField: "place", alertMinutes: 0))
    }
}
