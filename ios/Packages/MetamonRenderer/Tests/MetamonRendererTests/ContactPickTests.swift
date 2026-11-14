import Contacts
import MetamonDSL
import XCTest
@testable import MetamonRenderer

/// 連絡先から 1 人選ぶ口(#2217)。`CNContactPickerViewController` の中身ではなく、
/// 「選んだ 1 人(`CNContact`)から項目を取り出す」「取り出した値を field に入れる形にする」をテストする。
/// `CNContactStore` は一切触らない(= 許可ダイアログが出ない)。
final class ContactPickTests: XCTestCase {
    private func contact(given: String = "杏奈", family: String = "堺",
                         phones: [String] = [], emails: [String] = [],
                         birthday: DateComponents? = nil) -> CNContact {
        let c = CNMutableContact()
        c.givenName = given
        c.familyName = family
        c.phoneNumbers = phones.map { CNLabeledValue(label: CNLabelPhoneNumberMobile, value: CNPhoneNumber(stringValue: $0)) }
        c.emailAddresses = emails.map { CNLabeledValue(label: CNLabelHome, value: $0 as NSString) }
        c.birthday = birthday
        return c.copy() as! CNContact
    }

    private func peopleEntity() throws -> Entity {
        try JSONDecoder().decode(Entity.self, from: Data(#"""
        {"id":"people","name":"人","fields":[
          {"id":"title","name":"名前","type":"string"},
          {"id":"tel","name":"電話","type":"string"}]}
        """#.utf8))
    }

    private func day(_ year: Int?, _ month: Int, _ dayOfMonth: Int) -> DateComponents {
        var c = DateComponents()
        c.calendar = Calendar(identifier: .gregorian)
        c.year = year
        c.month = month
        c.day = dayOfMonth
        return c
    }

    // MARK: - 選んだ連絡先から項目を取り出す

    func testReadsRequestedItems() {
        let picked = ContactPick.read(
            contact(phones: ["090-1234-5678"], emails: ["anna@example.com"], birthday: day(2004, 4, 23)),
            items: Set(ContactItem.allCases)
        )
        XCTAssertEqual(picked[.phone], "090-1234-5678")
        XCTAssertEqual(picked[.email], "anna@example.com")
        XCTAssertEqual(picked[.birthday], "2004-04-23")
        XCTAssertEqual(picked[.name]?.contains("杏奈"), true)
        XCTAssertEqual(picked[.name]?.contains("堺"), true)
    }

    func testReadsOnlyRequestedItems() {
        let picked = ContactPick.read(
            contact(phones: ["090-1234-5678"], emails: ["anna@example.com"], birthday: day(2004, 4, 23)),
            items: [.phone]
        )
        XCTAssertEqual(picked.values.keys.map(\.self), [.phone])
        XCTAssertNil(picked[.name])
        XCTAssertNil(picked[.email])
        XCTAssertNil(picked[.birthday])
    }

    // MARK: - 複数ある時は最初の 1 つ

    func testTakesFirstPhoneAndEmailWhenSeveral() {
        let picked = ContactPick.read(
            contact(phones: ["090-1111-1111", "03-2222-2222", "080-3333-3333"],
                    emails: ["first@example.com", "second@example.com"]),
            items: [.phone, .email]
        )
        XCTAssertEqual(picked[.phone], "090-1111-1111")
        XCTAssertEqual(picked[.email], "first@example.com")
    }

    // MARK: - 項目が無い時は空

    func testMissingItemsAreEmpty() {
        let picked = ContactPick.read(contact(given: "", family: ""), items: Set(ContactItem.allCases))
        XCTAssertTrue(picked.isEmpty, "名前・電話・メール・誕生日が無ければ何も入らない")
        XCTAssertNil(picked[.phone])
        XCTAssertNil(picked[.email])
        XCTAssertNil(picked[.birthday])
        XCTAssertNil(picked[.name])
    }

    func testBlankValuesAreDropped() {
        let picked = ContactPick.read(contact(phones: ["  "], emails: ["\n"]), items: [.phone, .email])
        XCTAssertNil(picked[.phone])
        XCTAssertNil(picked[.email])
    }

    func testBirthdayWithoutYearIsEmpty() {
        let picked = ContactPick.read(contact(birthday: day(nil, 4, 23)), items: [.birthday])
        XCTAssertNil(picked[.birthday], "年の無い誕生日は date の値にできないので入れない")
    }

    // MARK: - 値をログに出さない

    func testDescriptionHidesValues() {
        let picked = ContactPick.read(contact(phones: ["090-1234-5678"]), items: [.phone])
        XCTAssertFalse("\(picked)".contains("090"))
        XCTAssertTrue("\(picked)".contains("phone"))
    }

    // MARK: - field に入れる形

    func testPrefillValuesMapsItemsToFields() {
        let picked = PickedContact(values: [.name: "堺 杏奈", .phone: "090-1234-5678", .birthday: "2004-04-23"])
        let values = ContactPick.prefillValues(picked, fields: [.name: "title", .phone: "tel", .email: "mail", .birthday: "bday"])
        XCTAssertEqual(values, ["title": .string("堺 杏奈"), "tel": .string("090-1234-5678"), "bday": .string("2004-04-23")])
        XCTAssertNil(values["mail"], "取れなかった項目は field にも入らない")
    }

    func testPrefillValuesIsEmptyWhenNothingPicked() {
        XCTAssertEqual(ContactPick.prefillValues(PickedContact(), fields: [.name: "title", .phone: "tel"]), [:])
        XCTAssertEqual(ContactPick.prefillValues(PickedContact(values: [.name: "堺 杏奈"]), fields: [:]), [:])
        XCTAssertEqual(ContactPick.prefillValues(PickedContact(values: [.name: "堺 杏奈"]), fields: [.name: ""]), [:])
    }

    @MainActor
    func testPrefillPushesValuesToTheNextForm() throws {
        let entity = try peopleEntity()
        let values = ContactPick.prefill(PickedContact(values: [.name: "堺 杏奈", .phone: "090-1234-5678"]),
                                         fields: [.name: "title", .phone: "tel"])
        XCTAssertEqual(values.count, 2)
        XCTAssertEqual(FormPrefill.consume(for: entity), ["title": .string("堺 杏奈"), "tel": .string("090-1234-5678")])
        XCTAssertEqual(FormPrefill.consume(for: entity), [:], "1 回だけ消費される")
    }

    @MainActor
    func testPrefillKeepsPendingValuesWhenNothingPicked() throws {
        let entity = try peopleEntity()
        FormPrefill.set(["title": .string("先に積んだ値")])
        XCTAssertEqual(ContactPick.prefill(PickedContact(), fields: [.name: "title"]), [:])
        XCTAssertEqual(FormPrefill.consume(for: entity), ["title": .string("先に積んだ値")], "空では上書きしない")
    }
}
