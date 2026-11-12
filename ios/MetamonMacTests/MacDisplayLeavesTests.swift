import XCTest
import AppKit
import MetamonDSL
@testable import MetamonRenderer
import MetamonServices
@testable import Metamon

/// Mac の表示 leaf・一覧 leaf(#941・#944): rating / flashcards / calendar のキー、SF Symbols の言い換え、⌘R の再読み込み。
@MainActor
final class MacDisplayLeavesTests: XCTestCase {
    // MARK: - rating のキー

    func testRatingArrowsStepWithinRange() {
        XCTAssertEqual(MacRatingKeys.value(after: .right, current: 2, count: 5), 3)
        XCTAssertEqual(MacRatingKeys.value(after: .up, current: 2, count: 5), 3)
        XCTAssertEqual(MacRatingKeys.value(after: .left, current: 2, count: 5), 1)
        XCTAssertEqual(MacRatingKeys.value(after: .down, current: 2, count: 5), 1)
        XCTAssertEqual(MacRatingKeys.value(after: .right, current: 5, count: 5), 5, "最大で止まる")
        XCTAssertEqual(MacRatingKeys.value(after: .left, current: 0, count: 5), 0, "0(未評価)で止まる")
    }

    func testRatingDigitsSetValueAndZeroClears() {
        XCTAssertEqual(MacRatingKeys.value(after: .digit(4), current: 1, count: 5), 4)
        XCTAssertEqual(MacRatingKeys.value(after: .digit(0), current: 3, count: 5), 0)
        XCTAssertEqual(MacRatingKeys.value(after: .digit(9), current: 1, count: 5), 5, "数より大きい数字は最大")
    }

    func testRatingIgnoresOtherKeysAndClampsBadCurrent() {
        XCTAssertNil(MacRatingKeys.value(after: .space, current: 1, count: 5))
        XCTAssertNil(MacRatingKeys.value(after: .other, current: 1, count: 5))
        XCTAssertEqual(MacRatingKeys.value(after: .right, current: 99, count: 5), 5)
    }

    func testRatingVoiceOverAdjust() {
        XCTAssertEqual(MacRatingKeys.adjusted(increment: true, current: 3, count: 5), 4)
        XCTAssertEqual(MacRatingKeys.adjusted(increment: false, current: 3, count: 5), 2)
        XCTAssertEqual(MacRatingKeys.adjusted(increment: true, current: 10, count: 10), 10)
    }

    // MARK: - flashcards のキー

    func testFlashcardKeys() {
        XCTAssertEqual(MacFlashcardKeys.command(for: .space), .flip)
        XCTAssertEqual(MacFlashcardKeys.command(for: .left), .notYet, "iOS の左スワイプと同じ")
        XCTAssertEqual(MacFlashcardKeys.command(for: .right), .learned, "iOS の右スワイプと同じ")
        XCTAssertNil(MacFlashcardKeys.command(for: .up))
        XCTAssertNil(MacFlashcardKeys.command(for: .digit(1)))
    }

    // MARK: - calendar

    func testCalendarArrowsMoveDayAndWeekAcrossMonths() throws {
        var cal = Calendar(identifier: .gregorian)
        cal.timeZone = TimeZone(identifier: "Asia/Tokyo")!
        let d = try XCTUnwrap(cal.date(from: DateComponents(year: 2026, month: 10, day: 31)))
        func day(_ key: MacLeafKey) -> DateComponents? {
            MacCalendarKeys.move(d, by: key, calendar: cal).map { cal.dateComponents([.month, .day], from: $0) }
        }
        XCTAssertEqual(day(.right), DateComponents(month: 11, day: 1), "月をまたぐ")
        XCTAssertEqual(day(.left), DateComponents(month: 10, day: 30))
        XCTAssertEqual(day(.down), DateComponents(month: 11, day: 7))
        XCTAssertEqual(day(.up), DateComponents(month: 10, day: 24))
        XCTAssertNil(MacCalendarKeys.move(d, by: .space, calendar: cal))
    }

    func testCalendarCellsGrowWithWindow() {
        XCTAssertEqual(MacCalendarLayout.cellHeight(gridWidth: 0), 36)
        XCTAssertEqual(MacCalendarLayout.cellHeight(gridWidth: 300), 36, "狭い時は iOS と同じ")
        XCTAssertEqual(MacCalendarLayout.cellHeight(gridWidth: 700), 75)
        XCTAssertEqual(MacCalendarLayout.cellHeight(gridWidth: 2000), 96, "上限")
        XCTAssertEqual(MacCalendarLayout.titleLines(cellHeight: 36), 0, "狭い枠は点だけ")
        XCTAssertEqual(MacCalendarLayout.titleLines(cellHeight: 75), 2)
        XCTAssertEqual(MacCalendarLayout.titleLines(cellHeight: 96), 3)
    }

    // MARK: - SF Symbols の言い換え

    private func exists(_ name: String) -> Bool { NSImage(systemSymbolName: name, accessibilityDescription: nil) != nil }

    func testSymbolAliasesAllExistOnMac() {
        for (from, to) in MacSymbolFallback.aliases {
            XCTAssertFalse(exists(from), "\(from) は Mac にあるので言い換え不要")
            XCTAssertTrue(exists(to), "\(from) → \(to) が Mac に無い")
        }
    }

    func testSymbolResolveUsesNameThenAliasThenFallback() {
        XCTAssertEqual(PlatformSymbol.name("star.fill", fallback: "circle"), "star.fill")
        XCTAssertEqual(PlatformSymbol.name("home", fallback: "circle"), "house")
        XCTAssertEqual(PlatformSymbol.name("Settings", fallback: "circle"), "gearshape", "大文字小文字を問わない")
        XCTAssertEqual(PlatformSymbol.name("no.such.symbol.xyz", fallback: "tray"), "tray")
        XCTAssertEqual(PlatformSymbol.name(nil, fallback: "questionmark.circle"), "questionmark.circle")
        XCTAssertEqual(PlatformSymbol.name("", fallback: "circle"), "circle")
        XCTAssertEqual(MacSymbolFallback.resolve("a", fallback: "f", exists: { $0 == "b" }), "f")
    }

    /// 入力・stat の左の記号の表(FieldIcon)が出す名前がすべて Mac で描けること
    func testFieldIconMapExistsOnMac() throws {
        let types = ["string", "text", "integer", "number", "boolean", "date", "datetime", "enum", "currency", "photo",
                     "location", "video", "audio", "ref", "refs", "member", "members"]
        let hints = ["", "time 時", "weight", "reps", "steps", "memo", "pin", "mail", "url", "phone", "name", "search"]
        var names = Set<String>()
        for t in types {
            let field = try JSONDecoder().decode(Field.self, from: Data(#"{"id":"f","name":"f","type":"\#(t)"}"#.utf8))
            for h in hints { names.insert(FieldIcon.infer(field: field, hint: h)) }
        }
        for h in hints { names.insert(FieldIcon.infer(field: nil, hint: h)) }
        for agg in ["count", "avg", "max", "min", "distance", "duration", nil] {
            names.insert(FieldIcon.stat(aggregate: agg, field: nil))
        }
        // 表示 leaf の既定の記号
        names.formUnion(["questionmark.circle", "circle", "tray", "arrow.up.right.square", "person.fill", "star", "star.fill",
                         "rectangle.on.rectangle.angled", "checkmark.seal.fill", "chevron.left", "chevron.right"])
        for n in names.sorted() { XCTAssertTrue(exists(n), "\(n) が Mac に無い") }
    }

    func testFieldIconReplacesUnknownDSLIconOnMac() throws {
        let field = try JSONDecoder().decode(Field.self, from: Data(#"{"id":"amount","name":"金額","type":"currency"}"#.utf8))
        var c = try JSONDecoder().decode(Component.self, from: Data(#"{"type":"numberField","field":"amount","icon":"money"}"#.utf8))
        XCTAssertEqual(FieldIcon.infer(component: c, field: field), "banknote")
        c.icon = "not.a.symbol.xyz"
        XCTAssertEqual(FieldIcon.infer(component: c, field: field), "yensign", "無い名前は型からの推定")
        c.icon = "cart"
        XCTAssertEqual(FieldIcon.infer(component: c, field: field), "cart")
    }

    // MARK: - ⌘R 再読み込み

    private func runtime() throws -> MacAppRuntime {
        let url = try XCTUnwrap(Bundle(for: MacAppRuntime.self).url(forResource: "kakeibo", withExtension: "json"))
        return MacAppRuntime(appId: "test-mac-\(UUID().uuidString.lowercased())", dsl: try AppDSL.load(contentsOf: url),
                             ai: MockAIClient(delay: .zero), isAIConsentGranted: { false })
    }

    func testReloadTargetsTopScreenEntities() async throws {
        let rt = try runtime()
        let home = try XCTUnwrap(rt.dsl.screen(rt.currentScreen))
        XCTAssertEqual(rt.menuReloadEntities, rt.dsl.entitiesNeeded(by: home))
        XCTAssertTrue(rt.menuReloadEntities.contains("expense"))
        XCTAssertTrue(rt.canReloadFromMenu)
        await rt.reloadFromMenu() // メモリ上の記録を読み直しても落ちない
        XCTAssertTrue(rt.canReloadFromMenu)
    }

    func testReloadDisabledWhileDialogShown() throws {
        let rt = try runtime()
        rt.confirm = RuntimeConfirm(message: "?", primaryLabel: "OK", isDestructive: false) {}
        XCTAssertFalse(rt.canReloadFromMenu, "確認ダイアログの間は押せない")
    }
}
