import XCTest
@testable import MetamonRenderer
@testable import Metamon

/// カレンダーに追加する前の確認に出す、アラームの前の時間の文(`MacCalendarSheet.alertText`)。
/// offset は始まりからの差(秒・負が前)。ja / en を表で確かめる
@MainActor
final class MacCalendarAlertTextTests: XCTestCase {
    private var savedLanguage: AppLanguage = .system

    override func setUp() {
        super.setUp()
        savedLanguage = LanguagePreference.shared.setting
    }

    override func tearDown() {
        LanguagePreference.shared.setting = savedLanguage
        super.tearDown()
    }

    func testJapanese() {
        LanguagePreference.shared.setting = .ja
        let table: [(TimeInterval, String)] = [
            (0, "始まる時に通知"),
            (-5 * 60, "始まる 5 分前に通知"),
            (-3600, "始まる 60 分前に通知"),
        ]
        for (offset, expected) in table {
            XCTAssertEqual(MacCalendarSheet.alertText(offset), expected, "offset=\(offset)")
        }
        // 1 日前は 1440 分前。桁区切り(1,440)は端末の地域設定で変わるので、前後だけ見る
        let day = MacCalendarSheet.alertText(-86_400)
        XCTAssertTrue(day.hasPrefix("始まる 1") && day.hasSuffix("440 分前に通知"), day)
    }

    func testEnglish() {
        LanguagePreference.shared.setting = .en
        let table: [(TimeInterval, String)] = [
            (0, "Alert at start time"),
            (-5 * 60, "Alert 5 min before"),
            (-3600, "Alert 60 min before"),
        ]
        for (offset, expected) in table {
            XCTAssertEqual(MacCalendarSheet.alertText(offset), expected, "offset=\(offset)")
        }
        let day = MacCalendarSheet.alertText(-86_400)
        XCTAssertTrue(day.hasPrefix("Alert 1") && day.hasSuffix("440 min before"), day)
    }

    /// 分に満たない端数は四捨五入(秒の揺れで文が変わらない)
    func testRoundsToNearestMinute() {
        LanguagePreference.shared.setting = .ja
        XCTAssertEqual(MacCalendarSheet.alertText(-299), "始まる 5 分前に通知")
        XCTAssertEqual(MacCalendarSheet.alertText(-20), "始まる時に通知")
    }
}
