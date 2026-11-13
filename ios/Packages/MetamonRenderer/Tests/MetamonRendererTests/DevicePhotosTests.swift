import Photos
import XCTest
@testable import MetamonRenderer

/// 写真ライブラリの読み取り口(#2219)。`PHPhotoLibrary` / `PHAsset` は実機依存なので触らず
/// (= 許可ダイアログが出ない)、「取得条件を組み立てる純粋関数」と「取得結果を整形する純粋関数」、
/// および偽物(`FakeDevicePhotos`)で、期間の絞り込み・一部だけ許可・許可なしを確かめる。
final class DevicePhotosTests: XCTestCase {
    private var cal: Calendar = {
        var c = Calendar(identifier: .gregorian)
        c.timeZone = TimeZone(identifier: "Asia/Tokyo")!
        return c
    }()
    private let now = ISO8601DateFormatter().date(from: "2026-11-12T09:30:00+09:00")!

    private func day(_ offset: Int, _ hour: Int = 0) -> Date {
        cal.date(byAdding: .hour, value: offset * 24 + hour, to: cal.startOfDay(for: now))!
    }

    private func asset(_ id: String, _ created: Date, kind: DevicePhotoKind = .image) -> DevicePhotoAsset {
        DevicePhotoAsset(id: id, created: created, kind: kind, hasLocation: false, pixelWidth: 100, pixelHeight: 100)
    }

    /// 今日・2 日前・5 日前・20 日前の 4 枚
    private var library: [DevicePhotoAsset] {
        [asset("today", day(0, 8)),
         asset("d2", day(-2, 19)),
         asset("d5", day(-5, 12), kind: .video),
         asset("d20", day(-20, 12))]
    }

    private func fake(access: DevicePhotoAccess = .authorized,
                      selectedIds: Set<String> = [],
                      assets: [DevicePhotoAsset]? = nil,
                      albums: [DevicePhotoAlbum] = [],
                      albumMembers: [String: Set<String>] = [:]) -> FakeDevicePhotos {
        let now = self.now
        return FakeDevicePhotos(access: access, assets: assets ?? library, selectedIds: selectedIds,
                                albums: albums, albumMembers: albumMembers, now: { now })
    }

    private func assertThrowsNotAuthorized(_ state: DevicePhotoAccess,
                                           _ body: () async throws -> Void,
                                           file: StaticString = #filePath, line: UInt = #line) async {
        do {
            try await body()
            XCTFail("\(state) で読めてしまった", file: file, line: line)
        } catch {
            XCTAssertEqual(error as? DevicePhotosError, .notAuthorized(state), file: file, line: line)
        }
    }

    // MARK: - 許可の状態

    func testAccessMapsEveryPhotoKitStatus() {
        XCTAssertEqual(DevicePhotoAccess(.notDetermined), .notDetermined)
        XCTAssertEqual(DevicePhotoAccess(.denied), .denied)
        XCTAssertEqual(DevicePhotoAccess(.restricted), .restricted)
        XCTAssertEqual(DevicePhotoAccess(.limited), .limited)
        XCTAssertEqual(DevicePhotoAccess(.authorized), .authorized)
    }

    func testOnlyAuthorizedAndLimitedCanRead() {
        XCTAssertTrue(DevicePhotoAccess.authorized.canRead)
        XCTAssertTrue(DevicePhotoAccess.limited.canRead, "一部だけ許可でも読める(見える範囲が狭いだけ)")
        for state in [DevicePhotoAccess.notDetermined, .denied, .restricted] {
            XCTAssertFalse(state.canRead, "\(state) で読めてはいけない")
        }
        XCTAssertTrue(DevicePhotoAccess.limited.isLimited)
        XCTAssertFalse(DevicePhotoAccess.authorized.isLimited)
    }

    // MARK: - 期間の絞り込み(取得条件の組み立て)

    func testFetchOptionsFiltersHalfOpenRangeNewestFirst() {
        let range = DateInterval(start: day(-7), end: day(1))
        let options = DevicePhotosQuery.fetchOptions(in: range)

        XCTAssertEqual(options.fetchLimit, DevicePhotosLimits.maxItems)
        XCTAssertFalse(options.includeHiddenAssets, "非表示の写真は読まない")
        XCTAssertEqual(options.sortDescriptors?.count, 1)
        XCTAssertEqual(options.sortDescriptors?.first?.key, DevicePhotosQuery.creationDateKey)
        XCTAssertEqual(options.sortDescriptors?.first?.ascending, false, "新しい順")

        // 組み立てた条件そのものを、撮影日を持つ値に当てて確かめる(半開区間)
        func matches(_ date: Date) -> Bool {
            options.predicate?.evaluate(with: [DevicePhotosQuery.creationDateKey: date]) ?? false
        }
        XCTAssertTrue(matches(day(-7)), "始まりの日時そのものは含む")
        XCTAssertTrue(matches(day(0, 8)))
        XCTAssertFalse(matches(day(1)), "終わりの日時そのものは含まない")
        XCTAssertFalse(matches(day(-8)), "期間より前は含まない")
        XCTAssertFalse(matches(day(2)), "期間より後は含まない")
    }

    func testFetchOptionsNeverTakesNegativeLimit() {
        XCTAssertEqual(DevicePhotosQuery.fetchOptions(in: DateInterval(start: day(-1), end: day(0)), limit: -5).fetchLimit, 0)
    }

    func testClampKeepsRangeInsideAllowedWindow() {
        let far = DevicePhotosLimits.clamp(from: day(-5000), to: day(5000), now: now, calendar: cal)
        let lower = cal.date(byAdding: .day, value: -DevicePhotosLimits.daysBack, to: cal.startOfDay(for: now))!
        let upper = cal.date(byAdding: .day, value: DevicePhotosLimits.daysAhead + 1, to: cal.startOfDay(for: now))!
        XCTAssertEqual(far?.start, lower)
        XCTAssertEqual(far?.end, upper)
    }

    func testClampKeepsRangeAsIsWhenInsideWindow() {
        let range = DevicePhotosLimits.clamp(from: day(-7), to: day(0), now: now, calendar: cal)
        XCTAssertEqual(range?.start, day(-7))
        XCTAssertEqual(range?.end, day(0))
    }

    func testClampIsNilWhenRangeIsEmptyOrReversed() {
        XCTAssertNil(DevicePhotosLimits.clamp(from: day(0), to: day(0), now: now, calendar: cal))
        XCTAssertNil(DevicePhotosLimits.clamp(from: day(0), to: day(-3), now: now, calendar: cal))
        XCTAssertNil(DevicePhotosLimits.clamp(from: day(-5000), to: day(-4000), now: now, calendar: cal),
                     "読める期間より前だけの指定は空")
    }

    func testRecentDaysCoversLastWeekIncludingToday() {
        let week = DevicePhotosLimits.recentDays(7, now: now, calendar: cal)
        XCTAssertEqual(week?.start, day(-7))
        XCTAssertEqual(week?.end, day(1), "今日の分まで入る")
        XCTAssertNil(DevicePhotosLimits.recentDays(0, now: now, calendar: cal))
        XCTAssertNil(DevicePhotosLimits.recentDays(-1, now: now, calendar: cal))
    }

    // MARK: - 期間の絞り込み(取得結果の整形)

    func testAssetsKeepOnlyItemsInRangeNewestFirst() {
        let got = DevicePhotosQuery.assets(library, in: DateInterval(start: day(-7), end: day(1)))
        XCTAssertEqual(got.map(\.id), ["today", "d2", "d5"], "20 日前は期間の外")
    }

    func testAssetsDropAssetsOutsideHalfOpenRange() {
        let edge = [asset("start", day(-7)), asset("end", day(1)), asset("inside", day(0))]
        let got = DevicePhotosQuery.assets(edge, in: DateInterval(start: day(-7), end: day(1)))
        XCTAssertEqual(Set(got.map(\.id)), ["start", "inside"], "終わりの日時そのものは含まない")
    }

    func testAssetsDropNonPhotoOrVideoItems() {
        let mixed = [asset("photo", day(0)), asset("movie", day(0), kind: .video), asset("audio", day(0), kind: .other)]
        let got = DevicePhotosQuery.assets(mixed, in: DateInterval(start: day(-1), end: day(1)))
        XCTAssertEqual(Set(got.map(\.id)), ["photo", "movie"])
    }

    func testAssetsCutAtLimitKeepingNewest() {
        let got = DevicePhotosQuery.assets(library, in: DateInterval(start: day(-30), end: day(1)), limit: 2)
        XCTAssertEqual(got.map(\.id), ["today", "d2"])
        XCTAssertTrue(DevicePhotosQuery.assets(library, in: DateInterval(start: day(-30), end: day(1)), limit: 0).isEmpty)
    }

    func testAssetsBreakTiesByIdSoOrderIsStable() {
        let same = [asset("b", day(0, 9)), asset("a", day(0, 9)), asset("c", day(0, 9))]
        XCTAssertEqual(DevicePhotosQuery.assets(same, in: DateInterval(start: day(-1), end: day(1))).map(\.id),
                       ["a", "b", "c"])
    }

    // MARK: - 一部だけ許可(limited)の時は選ばれた物だけ

    func testVisibleKeepsOnlySelectedWhenLimited() {
        let got = DevicePhotosQuery.visible(library, access: .limited, selectedIds: ["d2", "d20"])
        XCTAssertEqual(Set(got.map(\.id)), ["d2", "d20"], "ユーザーが選んだ写真だけが見える")
    }

    func testVisibleKeepsEverythingWhenFullyAuthorized() {
        let got = DevicePhotosQuery.visible(library, access: .authorized, selectedIds: ["d2"])
        XCTAssertEqual(got.count, library.count, "全部許可なら選択は関係ない")
    }

    func testVisibleIsEmptyWhenLimitedButNothingSelected() {
        XCTAssertTrue(DevicePhotosQuery.visible(library, access: .limited, selectedIds: []).isEmpty)
    }

    func testVisibleIsEmptyWithoutPermission() {
        for state in [DevicePhotoAccess.notDetermined, .denied, .restricted] {
            XCTAssertTrue(DevicePhotosQuery.visible(library, access: state, selectedIds: ["d2"]).isEmpty,
                          "\(state) では何も見えない")
        }
    }

    func testLimitedReadsOnlySelectedAssetsInRange() async throws {
        let f = fake(access: .limited, selectedIds: ["d2", "d20"])
        let week = try XCTUnwrap(DevicePhotosLimits.recentDays(7, now: now, calendar: cal))
        let got = try await f.assets(from: week.start, to: week.end, albumId: nil)
        XCTAssertEqual(got.map(\.id), ["d2"], "選ばれた物のうち、期間に入る 1 枚だけ")
    }

    func testLimitedReadsNothingWhenNothingSelected() async throws {
        let f = fake(access: .limited, selectedIds: [])
        let got = try await f.assets(from: day(-30), to: day(1), albumId: nil)
        XCTAssertTrue(got.isEmpty)
    }

    func testLimitedHidesAlbums() async throws {
        let trip = DevicePhotoAlbum(id: "trip", name: "旅行", count: 2)
        let f = fake(access: .limited, selectedIds: ["d2"], albums: [trip], albumMembers: ["trip": ["d2"]])
        let albums = try await f.albums()
        let inAlbum = try await f.assets(from: day(-30), to: day(1), albumId: "trip")
        XCTAssertTrue(albums.isEmpty, "一部だけ許可ではアルバムは見えない")
        XCTAssertTrue(inAlbum.isEmpty, "アルバムを指定しても空")
        XCTAssertTrue(DevicePhotosQuery.albums([trip], access: .limited).isEmpty)
    }

    func testLimitedThumbnailOnlyForSelectedAssets() async {
        let f = fake(access: .limited, selectedIds: ["d2"])
        let allowed = await f.thumbnail(for: "d2", maxPixel: DevicePhotosLimits.thumbnailMaxPixel)
        let blocked = await f.thumbnail(for: "today", maxPixel: DevicePhotosLimits.thumbnailMaxPixel)
        XCTAssertNotNil(allowed)
        XCTAssertNil(blocked, "選ばれていない写真のサムネイルは読めない")
    }

    // MARK: - 許可が無い時

    func testReadsWithoutPermissionThrowFixedErrorAndDoNotCrash() async {
        for state in [DevicePhotoAccess.notDetermined, .denied, .restricted] {
            let f = fake(access: state)
            await assertThrowsNotAuthorized(state) { _ = try await f.assets(from: self.day(-7), to: self.day(1), albumId: nil) }
            await assertThrowsNotAuthorized(state) { _ = try await f.assets(from: self.day(-7), to: self.day(1), albumId: "trip") }
            await assertThrowsNotAuthorized(state) { _ = try await f.albums() }
        }
    }

    func testThumbnailIsNilWithoutPermission() async {
        for state in [DevicePhotoAccess.notDetermined, .denied, .restricted] {
            let f = fake(access: state)
            let image = await f.thumbnail(for: "today", maxPixel: DevicePhotosLimits.thumbnailMaxPixel)
            XCTAssertNil(image, "\(state) ではサムネイルも読めない(落ちずに nil)")
        }
    }

    func testErrorHasMessageForEveryDeniedState() {
        for state in [DevicePhotoAccess.notDetermined, .denied, .restricted] {
            XCTAssertFalse(DevicePhotosError.notAuthorized(state).errorDescription?.isEmpty ?? true)
        }
    }

    func testAuthorizationOnlyAsksWhenNotDetermined() async {
        let asked = FakeDevicePhotos(access: .notDetermined, grantOnRequest: .limited)
        let silent = await asked.authorization(requesting: false)
        let granted = await asked.authorization(requesting: true)
        XCTAssertEqual(silent, .notDetermined, "求めなければ訊かない")
        XCTAssertEqual(granted, .limited)

        for state in [DevicePhotoAccess.denied, .restricted, .limited, .authorized] {
            let settled = FakeDevicePhotos(access: state, grantOnRequest: .authorized)
            let again = await settled.authorization(requesting: true)
            XCTAssertEqual(again, state, "一度決まった後は訊き直さない")
        }
    }

    /// 説明文の無いビルド(審査用の版)では許可を求めない = 落ちない
    func testUsageDescriptionIsRequiredBeforeAsking() {
        XCTAssertTrue(LiveDevicePhotos.hasUsageDescription(in: [LiveDevicePhotos.usageKey: "写真を読みます"]))
        XCTAssertFalse(LiveDevicePhotos.hasUsageDescription(in: [LiveDevicePhotos.usageKey: ""]))
        XCTAssertFalse(LiveDevicePhotos.hasUsageDescription(in: ["NSCameraUsageDescription": "カメラ"]))
        XCTAssertFalse(LiveDevicePhotos.hasUsageDescription(in: [:]))
        XCTAssertFalse(LiveDevicePhotos.hasUsageDescription(in: nil))
    }

    // MARK: - アルバムで読む

    func testReadsOnlyAssetsInGivenAlbum() async throws {
        let trip = DevicePhotoAlbum(id: "trip", name: "旅行", count: 2)
        let food = DevicePhotoAlbum(id: "food", name: "ごはん", count: 1)
        let f = fake(albums: [trip, food], albumMembers: ["trip": ["today", "d5"], "food": ["d2"]])
        let got = try await f.assets(from: day(-7), to: day(1), albumId: "trip")
        let foodAssets = try await f.assets(from: day(-7), to: day(1), albumId: "food")
        XCTAssertEqual(got.map(\.id), ["today", "d5"])
        XCTAssertEqual(foodAssets.map(\.id), ["d2"])
    }

    func testUnknownAlbumIsEmpty() async throws {
        let f = fake(albums: [], albumMembers: [:])
        let got = try await f.assets(from: day(-7), to: day(1), albumId: "no-such-album")
        XCTAssertTrue(got.isEmpty)
    }

    func testAlbumsAreSortedByName() async throws {
        let albums = [DevicePhotoAlbum(id: "c", name: "旅行", count: 1),
                      DevicePhotoAlbum(id: "a", name: "あさごはん", count: 2),
                      DevicePhotoAlbum(id: "b", name: "Family", count: 3)]
        let got = try await fake(albums: albums).albums()
        XCTAssertEqual(got.map(\.name), albums.map(\.name).sorted { $0.localizedStandardCompare($1) == .orderedAscending })
    }

    // MARK: - 「先週の写真」(実機で確かめるのと同じ道)

    func testLastWeekReturnsRecentAssetsNewestFirst() async throws {
        let week = try XCTUnwrap(DevicePhotosLimits.recentDays(7, now: now, calendar: cal))
        let got = try await fake().assets(from: week.start, to: week.end, albumId: nil)
        XCTAssertEqual(got.map(\.id), ["today", "d2", "d5"], "先週の写真が新しい順で返る")
    }

    func testSampleLibraryIsReadableForMockMode() async throws {
        let sample = FakeDevicePhotos.sample
        let access = await sample.authorization(requesting: false)
        XCTAssertEqual(access, .authorized)
        let got = try await sample.assets(from: Date(timeIntervalSinceNow: -7 * 24 * 60 * 60),
                                         to: Date(timeIntervalSinceNow: 60), albumId: nil)
        let albums = try await sample.albums()
        XCTAssertFalse(got.isEmpty)
        XCTAssertFalse(albums.isEmpty)
    }

    // MARK: - 中身を誤って漏らさない

    func testAssetDescriptionHidesDateAndLocation() {
        let a = DevicePhotoAsset(id: "secret-id", created: day(0), kind: .image, hasLocation: true)
        XCTAssertEqual(a.description, "DevicePhotoAsset(kind: image)")
        XCTAssertFalse(a.debugDescription.contains("secret-id"))
    }
}
