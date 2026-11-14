import XCTest
@testable import MetamonDSL

/// AppDocument.swift の小さな型(FieldType・NotificationSpec.Near・ToolbarButton・VoiceEntry)の読み方。
/// どれも Decodable だけなので、往復ではなく知らない値・欠けた鍵の扱いを確かめる。
final class AppDocumentSmallTypesTests: XCTestCase {
    private func decode<T: Decodable>(_ type: T.Type, _ json: String) throws -> T {
        try JSONDecoder().decode(type, from: Data(json.utf8))
    }

    // MARK: - FieldType

    func testFieldTypeKnownValues() throws {
        for raw in ["string", "text", "integer", "number", "boolean", "date", "datetime", "enum", "currency", "photo", "location", "video", "audio", "ref", "refs", "member", "members"] {
            XCTAssertEqual(try decode(FieldType.self, "\"\(raw)\"").rawValue, raw)
        }
    }

    func testFieldTypeUnknownFallsBackToString() throws {
        XCTAssertEqual(try decode(FieldType.self, #""hologram""#), .string)
        XCTAssertEqual(try decode(FieldType.self, #""""#), .string)
        XCTAssertEqual(try decode(FieldType.self, "42"), .string, "文字列でない値も落とさない")
        XCTAssertEqual(try decode(FieldType.self, "null"), .string)
    }

    func testFieldTypeNumericAndDateFlags() {
        XCTAssertTrue(FieldType.integer.isNumeric)
        XCTAssertTrue(FieldType.number.isNumeric)
        XCTAssertTrue(FieldType.currency.isNumeric)
        XCTAssertFalse(FieldType.string.isNumeric)
        XCTAssertFalse(FieldType.boolean.isNumeric)
        XCTAssertTrue(FieldType.date.isDate)
        XCTAssertTrue(FieldType.datetime.isDate)
        XCTAssertFalse(FieldType.integer.isDate)
    }

    func testFieldWithMissingOrUnknownTypeBecomesString() throws {
        XCTAssertEqual(try decode(Field.self, #"{"id":"a"}"#).type, .string)
        XCTAssertEqual(try decode(Field.self, #"{"id":"a","type":"hologram"}"#).type, .string)
        XCTAssertEqual(try decode(Field.self, #"{"id":"a"}"#).name, "a", "name が無ければ id")
    }

    // MARK: - NotificationSpec.Near

    private func spec(near: String) throws -> NotificationSpec {
        try decode(NotificationSpec.self, #"{"id":"n","title":"t","body":"b","repeat":"once","near":\#(near)}"#)
    }

    func testNearReadsKeysAndDefaultRadius() throws {
        let n = try XCTUnwrap(try spec(near: #"{"entity":"shop","locationField":"place"}"#).near)
        XCTAssertEqual(n, .init(entity: "shop", locationField: "place"))
        XCTAssertNil(n.radiusM)
        XCTAssertEqual(n.radius, Double(NotificationSpec.Near.defaultRadius))
    }

    func testNearRadiusClampedTo100Through2000() throws {
        func radius(_ m: Int) throws -> Double {
            try XCTUnwrap(try spec(near: #"{"entity":"e","locationField":"f","radiusM":\#(m)}"#).near).radius
        }
        XCTAssertEqual(try radius(50), 100)
        XCTAssertEqual(try radius(100), 100)
        XCTAssertEqual(try radius(500), 500)
        XCTAssertEqual(try radius(2000), 2000)
        XCTAssertEqual(try radius(99_999), 2000)
        XCTAssertEqual(try radius(-5), 100)
    }

    func testNearMissingKeyFailsToDecode() {
        XCTAssertThrowsError(try decode(NotificationSpec.Near.self, #"{"entity":"e"}"#))
        XCTAssertThrowsError(try decode(NotificationSpec.Near.self, #"{"locationField":"f"}"#))
    }

    func testNearBasedNotificationFlags() throws {
        let s = try spec(near: #"{"entity":"shop","locationField":"place"}"#)
        XCTAssertTrue(s.isNearBased)
        XCTAssertFalse(s.isRecordBased)
        XCTAssertEqual(s.recordEntity, "shop")
    }

    // MARK: - ToolbarButton

    func testToolbarButtonReadsLabelIconAction() throws {
        let b = try decode(ToolbarButton.self, #"{"label":"保存","icon":"checkmark","action":{"type":"back"}}"#)
        XCTAssertEqual(b.label, "保存")
        XCTAssertEqual(b.icon, "checkmark")
        XCTAssertEqual(b.action.type, .back)
        XCTAssertEqual(b.id, "保存checkmark")
    }

    func testToolbarButtonWithoutIconUsesLabelAsId() throws {
        let b = try decode(ToolbarButton.self, #"{"label":"戻る","action":{"type":"back"}}"#)
        XCTAssertNil(b.icon)
        XCTAssertEqual(b.id, "戻る")
    }

    func testToolbarButtonMissingLabelOrActionFails() {
        XCTAssertThrowsError(try decode(ToolbarButton.self, #"{"action":{"type":"back"}}"#))
        XCTAssertThrowsError(try decode(ToolbarButton.self, #"{"label":"x"}"#))
    }

    func testScreenDropsUnreadableToolbarButtonsOnly() throws {
        let s = try decode(Screen.self, #"{"id":"s","toolbar":[{"label":"ok","action":{"type":"back"}},{"icon":"x"}],"components":[]}"#)
        XCTAssertEqual(s.toolbar.map(\.label), ["ok"])
    }

    // MARK: - VoiceEntry

    func testVoiceEntryKnownValues() throws {
        XCTAssertEqual(try decode(VoiceEntry.self, #""fullscreen""#), .fullscreen)
        XCTAssertEqual(try decode(VoiceEntry.self, #""floating""#), .floating)
        XCTAssertEqual(try decode(VoiceEntry.self, #""none""#), .none)
    }

    func testVoiceEntryUnknownFallsBackToFloating() throws {
        XCTAssertEqual(try decode(VoiceEntry.self, #""sideways""#), .floating)
        XCTAssertEqual(try decode(VoiceEntry.self, "1"), .floating)
    }

    func testAppMetaVoiceEntryDefaultsToFloating() throws {
        XCTAssertEqual(try decode(AppMeta.self, #"{"name":"a"}"#).voiceEntry, .floating)
        XCTAssertEqual(try decode(AppMeta.self, #"{"name":"a","voiceEntry":"none"}"#).voiceEntry, .none)
        XCTAssertEqual(try decode(AppMeta.self, #"{"name":"a","voiceEntry":"bogus"}"#).voiceEntry, .floating)
    }
}
