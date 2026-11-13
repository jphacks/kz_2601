import Contacts
import XCTest
@testable import MetamonRenderer

/// 端末の連絡先の一覧を読む口(#2218)。許可・limited・誕生日の変換・並びを偽物で確かめる。
final class DeviceContacts2218Tests: XCTestCase {
    private let family = DeviceContactGroup(id: "g-family", name: "家族")
    private let friends = DeviceContactGroup(id: "g-friends", name: "Friends")

    private var people: [DeviceContact] {
        [
            DeviceContact(id: "b", name: "  佐藤 太郎 ", birthday: DeviceContactBirthday(year: nil, month: 11, day: 3)),
            DeviceContact(id: "a", name: "Aoki", birthday: DeviceContactBirthday(year: 1990, month: 4, day: 12), groupIds: ["g-family"]),
            DeviceContact(id: "c", name: "", birthday: nil),
        ]
    }

    // MARK: 許可

    func testAccessMapsContactsStatus() {
        XCTAssertEqual(DeviceContactsAccess(.notDetermined), .notDetermined)
        XCTAssertEqual(DeviceContactsAccess(.denied), .denied)
        XCTAssertEqual(DeviceContactsAccess(.restricted), .restricted)
        XCTAssertEqual(DeviceContactsAccess(.authorized), .authorized)
        if #available(iOS 18.0, macOS 15.0, *) {
            XCTAssertEqual(DeviceContactsAccess(.limited), .limited)
        }
        XCTAssertTrue(DeviceContactsAccess.authorized.canRead)
        XCTAssertTrue(DeviceContactsAccess.limited.canRead)
        XCTAssertFalse(DeviceContactsAccess.denied.canRead)
        XCTAssertFalse(DeviceContactsAccess.notDetermined.canRead)
    }

    func testReadsWithoutAccessThrow() async {
        for state in [DeviceContactsAccess.notDetermined, .denied, .restricted] {
            let f = FakeDeviceContacts(access: state, contacts: people, groups: [family])
            do {
                _ = try await f.contacts()
                XCTFail("\(state) で読めてしまった")
            } catch {
                XCTAssertEqual(error as? DeviceContactsError, .notAuthorized(state))
            }
            do {
                _ = try await f.groups()
                XCTFail("\(state) でグループが読めてしまった")
            } catch {
                XCTAssertEqual(error as? DeviceContactsError, .notAuthorized(state))
            }
        }
    }

    func testRequestingOnlyChangesUndetermined() async {
        let undecided = FakeDeviceContacts(access: .notDetermined, grantOnRequest: .limited)
        let a1 = await undecided.authorization(requesting: false)
        let a2 = await undecided.authorization(requesting: true)
        XCTAssertEqual(a1, .notDetermined)
        XCTAssertEqual(a2, .limited)
        let denied = FakeDeviceContacts(access: .denied)
        let a3 = await denied.authorization(requesting: true)
        XCTAssertEqual(a3, .denied)
    }

    func testLimitedReturnsOnlyAllowedPeople() async throws {
        let f = FakeDeviceContacts(access: .limited, contacts: people, limitedIds: ["b"])
        let got = try await f.contacts()
        XCTAssertEqual(got.map(\.id), ["b"])
    }

    // MARK: 誕生日

    func testBirthdayWithYear() {
        let b = DeviceContactsQuery.birthday(DateComponents(year: 1990, month: 4, day: 12))
        XCTAssertEqual(b, DeviceContactBirthday(year: 1990, month: 4, day: 12))
        XCTAssertEqual(b?.isoString, "1990-04-12")
    }

    func testBirthdayWithoutYear() {
        let missing = DeviceContactsQuery.birthday(DateComponents(month: 11, day: 3))
        XCTAssertEqual(missing, DeviceContactBirthday(year: nil, month: 11, day: 3))
        XCTAssertEqual(missing?.isoString, "--11-03")
        let undefined = DeviceContactsQuery.birthday(DateComponents(year: NSDateComponentUndefined, month: 2, day: 29))
        XCTAssertNil(undefined?.year)
        XCTAssertEqual(undefined?.isoString, "--02-29")
    }

    func testNoBirthday() {
        XCTAssertNil(DeviceContactsQuery.birthday(nil))
        XCTAssertNil(DeviceContactsQuery.birthday(DateComponents(year: 2000)))
        XCTAssertNil(DeviceContactsQuery.birthday(DateComponents(month: 13, day: 1)))
    }

    // MARK: 並び・グループ

    func testSortedTrimsNamesAndPutsNamelessLast() async throws {
        let f = FakeDeviceContacts(contacts: people, groups: [family, friends])
        let got = try await f.contacts()
        XCTAssertEqual(got.map(\.id), ["a", "b", "c"])
        XCTAssertEqual(got[1].name, "佐藤 太郎")
        let groups = try await f.groups()
        XCTAssertEqual(groups.map(\.id), ["g-friends", "g-family"])
    }

    func testSortedCapsAtLimit() {
        let many = (0..<(DeviceContactsLimits.maxItems + 5)).map { DeviceContact(id: "\($0)", name: "n\($0)", birthday: nil) }
        XCTAssertEqual(DeviceContactsQuery.sorted(many).count, DeviceContactsLimits.maxItems)
    }

    func testGroupMembershipInverted() {
        let byContact = DeviceContactsQuery.groupIds(byContact: ["g2": ["p1"], "g1": ["p1", "p2"]])
        XCTAssertEqual(byContact["p1"], ["g1", "g2"])
        XCTAssertEqual(byContact["p2"], ["g1"])
        XCTAssertNil(byContact["p3"])
    }

    func testDescriptionHidesNameAndBirthday() {
        let c = DeviceContact(id: "x", name: "秘密の名前", birthday: DeviceContactBirthday(year: 1999, month: 1, day: 2))
        XCTAssertFalse("\(c)".contains("秘密"))
        XCTAssertFalse(String(reflecting: c).contains("1999"))
    }

    func testUsageDescriptionCheckUsesRawPlist() {
        XCTAssertFalse(LiveDeviceContacts.hasUsageDescription(in: nil))
        XCTAssertFalse(LiveDeviceContacts.hasUsageDescription(in: ["NSContactsUsageDescription": ""]))
        XCTAssertTrue(LiveDeviceContacts.hasUsageDescription(in: ["NSContactsUsageDescription": "読む"]))
    }
}
