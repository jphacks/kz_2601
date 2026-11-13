import XCTest
import MetamonDSL
import MetamonRenderer
@testable import Metamon

/// #3243: 保存の連打を 1 回にする `SaveFlight`
@MainActor
final class SaveFlight3243Tests: XCTestCase {
    private func form() throws -> FormDraft {
        let entity = try JSONDecoder().decode(Entity.self, from: Data(#"""
        {"id":"memo","name":"メモ","fields":[{"id":"title","name":"タイトル","type":"string"}]}
        """#.utf8))
        return FormDraft(entity: entity)
    }

    func testSecondBeginOnSameFormIsRejected() throws {
        let flight = SaveFlight()
        let f = try form()
        XCTAssertTrue(flight.begin(f))
        XCTAssertTrue(flight.isSaving(f))
        XCTAssertFalse(flight.begin(f), "連打の 2 回目は弾く")
    }

    func testDifferentFormsAreIndependent() throws {
        let flight = SaveFlight()
        let a = try form(), b = try form()
        XCTAssertTrue(flight.begin(a))
        XCTAssertTrue(flight.begin(b))
        flight.end(a)
        XCTAssertFalse(flight.isSaving(a))
        XCTAssertTrue(flight.isSaving(b))
    }

    func testBeginAgainAfterEnd() throws {
        let flight = SaveFlight()
        let f = try form()
        XCTAssertTrue(flight.begin(f))
        flight.end(f)
        XCTAssertFalse(flight.isSaving(f))
        XCTAssertTrue(flight.begin(f))
    }

    func testEndWithoutBeginIsHarmless() throws {
        let flight = SaveFlight()
        let f = try form()
        flight.end(f)
        XCTAssertFalse(flight.isSaving(f))
        XCTAssertTrue(flight.begin(f))
    }
}
