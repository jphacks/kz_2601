import XCTest
import MetamonDSL
@testable import MetamonRenderer

/// #150: 金額の「¥ 800 円」二重表示と、はい / いいえ の英訳。
final class MoneyDisplayTests: XCTestCase {
    private var saved: AppLanguage = .system

    override func setUp() { saved = LanguagePreference.shared.setting }
    override func tearDown() { LanguagePreference.shared.setting = saved }

    private func field(_ json: String) throws -> Field {
        try JSONDecoder().decode(Field.self, from: Data(json.utf8))
    }

    func testBooleanFollowsUILanguage() throws {
        let b = try field(#"{"id":"paid","name":"Paid","type":"boolean"}"#)
        LanguagePreference.shared.setting = .en
        XCTAssertEqual(displayText(.bool(true), field: b), "Yes")
        XCTAssertEqual(displayText(.bool(false), field: b), "No")
        LanguagePreference.shared.setting = .ja
        XCTAssertEqual(displayText(.bool(true), field: b), "はい")
        XCTAssertEqual(displayText(.bool(false), field: b), "いいえ")
    }

    func testNumberFieldUnitText() {
        XCTAssertNil(NumberFieldLeaf.unitText("円", isCurrency: true))
        XCTAssertNil(NumberFieldLeaf.unitText("JPY", isCurrency: true))
        XCTAssertEqual(NumberFieldLeaf.unitText("円/時", isCurrency: true), "/時")
        // currency 以外は従来どおり unit をそのまま出す
        XCTAssertEqual(NumberFieldLeaf.unitText("円", isCurrency: false), "円")
        XCTAssertEqual(NumberFieldLeaf.unitText("kg", isCurrency: false), "kg")
    }
}
