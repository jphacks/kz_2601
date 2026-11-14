import XCTest
import UIKit
@testable import Metamon

/// #232: ホーム画面のアイコンを選ぶ画面のモデル。
@MainActor
final class AppIconPickerTests: XCTestCase {
    private final class FakeSetting: AlternateIconSetting {
        var supportsAlternateIcons = true
        var alternateIconName: String?
        var error: Error?
        private(set) var calls: [String?] = []

        init(current: String? = nil) { alternateIconName = current }

        func setAlternateIconName(_ name: String?) async throws {
            calls.append(name)
            if let error { throw error }
            alternateIconName = name
        }
    }

    func testOptionsStartWithDefaultThenAlternates() {
        let options = AppIconOption.all
        XCTAssertEqual(options.map(\.id), ["AppIcon", "AppIcon-Dark", "AppIcon-Light", "AppIcon-Mono"])
        XCTAssertNil(options[0].alternateName, "先頭はいつものアイコン")
        XCTAssertEqual(Set(options.map(\.id)).count, options.count)
        for o in options { XCTAssertFalse(o.title.isEmpty) }
        XCTAssertEqual(options.map(\.previewName), ["AppIconPreview", "AppIconPreview-Dark", "AppIconPreview-Light", "AppIconPreview-Mono"])
    }

    /// project.yml の ASSETCATALOG_COMPILER_ALTERNATE_APPICON_NAMES と候補がずれていないか(actool が書いた Info.plist で確かめる)
    func testAlternatesMatchBundleInfoPlist() throws {
        let icons = try XCTUnwrap(Bundle.main.object(forInfoDictionaryKey: "CFBundleIcons") as? [String: Any])
        let alternates = try XCTUnwrap(icons["CFBundleAlternateIcons"] as? [String: Any])
        let expected = AppIconOption.all.compactMap(\.alternateName)
        XCTAssertEqual(Set(alternates.keys), Set(expected))
    }

    func testPreviewImagesAreBundled() {
        for o in AppIconOption.all {
            XCTAssertNotNil(UIImage(named: o.previewName), "\(o.previewName) が読めない(scripts/app-icon-previews.sh)")
        }
    }

    func testCurrentSelectionFollowsSystem() {
        XCTAssertEqual(AppIconPickerModel(setting: FakeSetting()).currentID, "AppIcon")
        XCTAssertEqual(AppIconPickerModel(setting: FakeSetting(current: "AppIcon-Mono")).currentID, "AppIcon-Mono")
        XCTAssertEqual(AppIconPickerModel(setting: FakeSetting(current: "Gone")).currentID, "AppIcon", "知らない名前はいつもの扱い")
    }

    func testSelectChangesIcon() async {
        let fake = FakeSetting()
        let model = AppIconPickerModel(setting: fake)
        let dark = AppIconOption.all[1]
        await model.select(dark)
        XCTAssertEqual(fake.calls, ["AppIcon-Dark"])
        XCTAssertTrue(model.isCurrent(dark))
        XCTAssertFalse(model.isCurrent(AppIconOption.all[0]))

        await model.select(AppIconOption.all[0])
        XCTAssertEqual(fake.calls, ["AppIcon-Dark", nil], "いつもの = nil")
        XCTAssertEqual(model.currentID, "AppIcon")
    }

    func testSelectingCurrentDoesNothing() async {
        let fake = FakeSetting(current: "AppIcon-Light")
        let model = AppIconPickerModel(setting: fake)
        await model.select(AppIconOption.all[2])
        XCTAssertTrue(fake.calls.isEmpty)
    }

    func testFailureKeepsCurrentAndShowsError() async {
        let fake = FakeSetting()
        fake.error = CocoaError(.featureUnsupported)
        let model = AppIconPickerModel(setting: fake)
        await model.select(AppIconOption.all[3])
        XCTAssertEqual(model.currentID, "AppIcon")
        XCTAssertNotNil(model.errorMessage)
        XCTAssertFalse(model.isChanging)
    }

    func testUnsupportedDeviceShowsError() async {
        let fake = FakeSetting()
        fake.supportsAlternateIcons = false
        let model = AppIconPickerModel(setting: fake)
        XCTAssertFalse(model.isSupported)
        await model.select(AppIconOption.all[1])
        XCTAssertTrue(fake.calls.isEmpty)
        XCTAssertNotNil(model.errorMessage)
    }
}
