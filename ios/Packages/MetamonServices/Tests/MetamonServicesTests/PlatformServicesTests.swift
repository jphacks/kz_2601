import XCTest
#if os(iOS)
import CoreLocation
#endif
@testable import MetamonServices

/// 場所の通知の位置の許可(#1049): iOS は「使用中のみ / 常に」で可。Mac は場所の通知を出さない(許可は常に無し)。
final class RegionAccessPlatformTests: XCTestCase {
    #if os(iOS)
    func testIsGrantedOnIOS() {
        XCTAssertTrue(RegionLocationAccess.isGranted(.authorizedWhenInUse))
        XCTAssertTrue(RegionLocationAccess.isGranted(.authorizedAlways))
        XCTAssertFalse(RegionLocationAccess.isGranted(.denied))
        XCTAssertFalse(RegionLocationAccess.isGranted(.restricted))
        XCTAssertFalse(RegionLocationAccess.isGranted(.notDetermined))
    }
    #endif

    #if os(macOS)
    func testSystemAccessIsOffOnMac() async {
        let access = RegionAccess.system
        let authorized = await access.isAuthorized()
        let asked = await access.authorize()
        let here = await access.lastKnown()
        XCTAssertFalse(authorized)
        XCTAssertFalse(asked, "Mac は位置の許可を聞かない")
        XCTAssertNil(here)
    }
    #endif
}

#if os(macOS)
/// Mac の音声入力の診断(`MacAudioInput.fill`・#1049)。装置の有無は実行環境しだい(CI は無いことがある)。
final class MacAudioInputTests: XCTestCase {
    private var saved: UserDefaults!
    private var suite: String!

    override func setUp() {
        super.setUp()
        saved = MacAudioInput.defaults
        suite = "MacAudioInputTests.\(UUID().uuidString)"
        MacAudioInput.defaults = UserDefaults(suiteName: suite)!
    }

    override func tearDown() {
        MacAudioInput.defaults.removePersistentDomain(forName: suite)
        MacAudioInput.defaults = saved
        super.tearDown()
    }

    func testFillWithoutSelectionUsesSystemDefault() {
        var diag = TranscriberDiagnostics()
        MacAudioInput.fill(&diag)
        XCTAssertEqual(diag.sessionCategory, "macOS")
        XCTAssertEqual(diag.sessionMode, "default")
        XCTAssertGreaterThanOrEqual(diag.inputSampleRate, 0)
        if let id = MacAudioInput.systemDefaultID(), let device = MacAudioInput.devices().first(where: { $0.id == id }) {
            XCTAssertTrue(diag.inputAvailable)
            XCTAssertEqual(diag.inputRoute, device.name)
        }
        if !diag.inputAvailable { XCTAssertEqual(diag.inputRoute, "", "入力が無ければ経路は空") }
    }

    func testFillWithMissingDeviceFallsBackButKeepsChoice() {
        MacAudioInput.preferredUID = "missing-\(UUID().uuidString)"
        MacAudioInput.preferredName = "外したマイク"
        var diag = TranscriberDiagnostics()
        MacAudioInput.fill(&diag)
        XCTAssertEqual(diag.sessionMode, "default", "見つからない装置は既定に戻る")
        XCTAssertNotEqual(diag.inputRoute, "外したマイク")
        XCTAssertNotNil(MacAudioInput.preferredUID, "選んだ値は消さない(挿し直せばまた使う)")
        XCTAssertEqual(MacAudioInput.preferredName, "外したマイク")
    }

    func testFillWithSelectedDeviceReportsIt() throws {
        guard let device = MacAudioInput.devices().first else { throw XCTSkip("入力の装置が無い環境") }
        MacAudioInput.preferredUID = device.uid
        var diag = TranscriberDiagnostics()
        MacAudioInput.fill(&diag)
        XCTAssertEqual(diag.sessionMode, "selected")
        XCTAssertEqual(diag.inputRoute, device.name)
        XCTAssertTrue(diag.inputAvailable)
        XCTAssertEqual(MacAudioInput.selectedDevice(), device)
    }

    func testClearingPreferenceRemovesName() {
        MacAudioInput.preferredUID = "uid-1"
        MacAudioInput.preferredName = "USB マイク"
        MacAudioInput.preferredUID = ""
        XCTAssertNil(MacAudioInput.preferredUID, "空は未選択")
        XCTAssertNil(MacAudioInput.preferredName)
        MacAudioInput.preferredUID = "uid-2"
        MacAudioInput.preferredUID = nil
        XCTAssertNil(MacAudioInput.defaults.string(forKey: MacAudioInput.preferenceKey))
    }
}
#endif
