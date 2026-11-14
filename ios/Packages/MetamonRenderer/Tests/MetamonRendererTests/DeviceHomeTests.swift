import XCTest
@testable import MetamonRenderer

/// HomeKit の家・部屋・機器を読む口(#2245)。偽物で、許可・家が無い時・並び・件数・文字への変換を確かめる。
final class DeviceHomeTests: XCTestCase {
    private let main = DeviceHomeInfo(id: "h1", name: "自宅", isPrimary: true)
    private let villa = DeviceHomeInfo(id: "h2", name: "別荘", isPrimary: false)

    private var living: DeviceRoom { DeviceRoom(id: "r1", name: "リビング", homeId: main.id) }
    private var bedroom: DeviceRoom { DeviceRoom(id: "r2", name: "寝室", homeId: main.id) }
    private var garage: DeviceRoom { DeviceRoom(id: "r3", name: "ガレージ", homeId: villa.id) }

    private func accessory(_ id: String, _ name: String, kind: DeviceAccessoryKind = .light,
                           home: DeviceHomeInfo? = nil, room: DeviceRoom? = nil,
                           readings: [DeviceHomeReading] = []) -> DeviceAccessory {
        DeviceAccessory(id: id, name: name, kind: kind, homeId: room?.homeId ?? home?.id ?? main.id,
                        roomId: room?.id, roomName: room?.name, isReachable: true, readings: readings)
    }

    private func fake(access: DeviceHomeAccess = .authorized,
                      homes: [DeviceHomeInfo]? = nil,
                      rooms: [DeviceRoom]? = nil,
                      accessories: [DeviceAccessory] = []) -> FakeDeviceHome {
        FakeDeviceHome(access: access, homes: homes ?? [main, villa],
                       rooms: rooms ?? [living, bedroom, garage], accessories: accessories)
    }

    // MARK: 許可

    func testReadsWithoutAuthorizationThrowFixedError() async {
        for state in [DeviceHomeAccess.notDetermined, .restricted] {
            let f = fake(access: state, accessories: [accessory("a", "ライト", room: living)])
            await XCTAssertThrowsAsync(DeviceHomeError.notAuthorized(state)) { _ = try await f.homes() }
            await XCTAssertThrowsAsync(DeviceHomeError.notAuthorized(state)) { _ = try await f.rooms(homeId: nil) }
            await XCTAssertThrowsAsync(DeviceHomeError.notAuthorized(state)) { _ = try await f.accessories(homeId: nil, roomId: nil) }
        }
    }

    func testRequestingOnlyChangesNotDetermined() async {
        var f = fake(access: .notDetermined)
        f.grantOnRequest = .authorized
        let notAsked = await f.authorization(requesting: false)
        XCTAssertEqual(notAsked, .notDetermined)
        let asked = await f.authorization(requesting: true)
        XCTAssertEqual(asked, .authorized)
        var restricted = fake(access: .restricted)
        restricted.grantOnRequest = .authorized
        let stillRestricted = await restricted.authorization(requesting: true) // 制限はダイアログを出さない
        XCTAssertEqual(stillRestricted, .restricted)
        XCTAssertTrue(DeviceHomeAccess.authorized.canRead)
        XCTAssertFalse(DeviceHomeAccess.notDetermined.canRead)
    }

    func testErrorMessagesExist() {
        for e in [DeviceHomeError.notAuthorized(.restricted), .unavailable, .noHome] {
            XCTAssertFalse((e.errorDescription ?? "").isEmpty)
        }
        XCTAssertNotEqual(DeviceHomeError.unavailable.errorDescription, DeviceHomeError.noHome.errorDescription)
    }

    // MARK: 家が無い

    func testNoHomeThrowsInsteadOfEmpty() async throws {
        let f = fake(homes: [], rooms: [], accessories: [])
        let homes = try await f.homes()
        XCTAssertTrue(homes.isEmpty)
        await XCTAssertThrowsAsync(DeviceHomeError.noHome) { _ = try await f.rooms(homeId: nil) }
        await XCTAssertThrowsAsync(DeviceHomeError.noHome) { _ = try await f.accessories(homeId: nil, roomId: nil) }
    }

    func testUnknownHomeIdIsTreatedAsNoHome() async {
        let f = fake()
        await XCTAssertThrowsAsync(DeviceHomeError.noHome) { _ = try await f.rooms(homeId: "nope") }
    }

    func testLiveSourceWithoutHomeKitIsUnavailable() async {
        #if !os(iOS)
        let live = LiveDeviceHome()
        let access = await live.authorization(requesting: true)
        XCTAssertEqual(access, .restricted)
        await XCTAssertThrowsAsync(DeviceHomeError.unavailable) { _ = try await live.homes() }
        #endif
    }

    // MARK: 並び・絞り込み

    func testHomesPutPrimaryFirstThenName() async throws {
        let other = DeviceHomeInfo(id: "h0", name: "あずかりもの", isPrimary: false)
        let got = try await fake(homes: [villa, other, main]).homes()
        XCTAssertEqual(got.map(\.id), [main.id, other.id, villa.id])
    }

    func testRoomsAndAccessoriesDefaultToPrimaryHome() async throws {
        let f = fake(accessories: [
            accessory("a1", "ライト", room: living),
            accessory("a2", "ヒーター", kind: .airConditioner, room: garage),
        ])
        let mainRooms = try await f.rooms(homeId: nil)
        XCTAssertEqual(mainRooms.map(\.id), [living.id, bedroom.id])
        let mainAccessories = try await f.accessories(homeId: nil, roomId: nil)
        XCTAssertEqual(mainAccessories.map(\.id), ["a1"])
        let villaRooms = try await f.rooms(homeId: villa.id)
        XCTAssertEqual(villaRooms.map(\.id), [garage.id])
        let villaAccessories = try await f.accessories(homeId: villa.id, roomId: nil)
        XCTAssertEqual(villaAccessories.map(\.id), ["a2"])
    }

    func testAccessoriesSortByRoomThenNameAndRoomlessComeLast() async throws {
        let f = fake(accessories: [
            accessory("roomless", "物置のライト"),
            accessory("bedroom", "ライト", room: bedroom),
            accessory("livingB", "ライト", room: living),
            accessory("livingA", "エアコン", kind: .airConditioner, room: living),
        ])
        let got = try await f.accessories(homeId: main.id, roomId: nil)
        XCTAssertEqual(got.map(\.id), ["livingA", "livingB", "bedroom", "roomless"])
        let onlyLiving = try await f.accessories(homeId: main.id, roomId: living.id)
        XCTAssertEqual(onlyLiving.map(\.id), ["livingA", "livingB"])
    }

    func testAccessoriesAreCappedAtMaxItems() async throws {
        let many = (0..<(DeviceHomeLimits.maxItems + 50)).map { i in
            accessory("a\(i)", String(format: "%04d", i), room: living)
        }
        let got = try await fake(accessories: many.shuffled()).accessories(homeId: main.id, roomId: nil)
        XCTAssertEqual(got.count, DeviceHomeLimits.maxItems)
        XCTAssertEqual(got.first?.name, "0000")
    }

    func testReadingsAreOrderedAndDeduplicated() {
        let got = DeviceHomeQuery.readings([
            DeviceHomeReading(trait: .humidity, value: 48),
            DeviceHomeReading(trait: .power, value: 1),
            DeviceHomeReading(trait: .humidity, value: 60),
            DeviceHomeReading(trait: .temperature, value: 21),
        ])
        XCTAssertEqual(got, [
            DeviceHomeReading(trait: .power, value: 1),
            DeviceHomeReading(trait: .temperature, value: 21),
            DeviceHomeReading(trait: .humidity, value: 48),
        ])
    }

    // MARK: 文字への変換

    func testEveryKindAndTraitHasALabel() {
        let kinds = DeviceAccessoryKind.allCases.map(\.label)
        XCTAssertEqual(kinds.count, Set(kinds).count)
        XCTAssertFalse(kinds.contains { $0.isEmpty })
        let traits = DeviceHomeTrait.allCases.map(\.label)
        XCTAssertEqual(traits.count, Set(traits).count)
        XCTAssertFalse(traits.contains { $0.isEmpty })
    }

    func testReadingTextPerTrait() {
        XCTAssertEqual(DeviceHomeReading(trait: .power, value: 1).text, L("オン"))
        XCTAssertEqual(DeviceHomeReading(trait: .power, value: 0).text, L("オフ"))
        XCTAssertEqual(DeviceHomeReading(trait: .brightness, value: 69.6).text, "70%")
        XCTAssertEqual(DeviceHomeReading(trait: .humidity, value: 48).text, "48%")
        XCTAssertEqual(DeviceHomeReading(trait: .temperature, value: 21.34).text, "21.3°C")
        XCTAssertEqual(DeviceHomeReading(trait: .temperature, value: -3).text, "-3.0°C")
    }

    /// 家・部屋・機器の名前をログに出さない
    func testDescriptionsHideNames() {
        let a = accessory("a1", "寝室のライト", room: bedroom)
        XCTAssertFalse(a.description.contains("寝室のライト"))
        XCTAssertFalse(bedroom.description.contains("寝室"))
        XCTAssertFalse(main.description.contains("自宅"))
    }

    // MARK: 固定の中身

    func testSampleHasRoomsAndReadings() async throws {
        let s = FakeDeviceHome.sample
        let homes = try await s.homes()
        XCTAssertEqual(homes.count, 1)
        let rooms = try await s.rooms(homeId: nil)
        XCTAssertEqual(rooms.count, 2)
        let all = try await s.accessories(homeId: nil, roomId: nil)
        XCTAssertEqual(all.count, 3)
        XCTAssertTrue(all.allSatisfy { !$0.readings.isEmpty })
    }

    private func XCTAssertThrowsAsync(_ expected: DeviceHomeError, file: StaticString = #filePath, line: UInt = #line,
                                      _ body: () async throws -> Void) async {
        do {
            try await body()
            XCTFail("throw しなかった", file: file, line: line)
        } catch {
            XCTAssertEqual(error as? DeviceHomeError, expected, file: file, line: line)
        }
    }
}
