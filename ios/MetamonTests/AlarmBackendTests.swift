import XCTest
import MetamonServices
@testable import Metamon

/// 止めるまで鳴るアラームの実装の選び方と、AlarmKit の時刻への写し。
final class AlarmBackendTests: XCTestCase {
    func testBackendOnlyOnIOS26_1() {
        #if APP_STORE_REVIEW
        XCTAssertNil(AlarmBackend.make(), "審査用の版はアラームを使わない(普通の通知で届ける)")
        #else
        if #available(iOS 26.1, *) {
            XCTAssertNotNil(AlarmBackend.make())
        } else {
            XCTAssertNil(AlarmBackend.make(), "iOS 26.1 未満は普通の通知に落とす")
        }
        #endif
    }

    #if !APP_STORE_REVIEW && canImport(AlarmKit)
    func testScheduleMapping() throws {
        guard #available(iOS 26.1, *) else { throw XCTSkip("AlarmKit は iOS 26.1 以降") }
        let date = Date(timeIntervalSince1970: 1_800_000_000)
        XCTAssertEqual(AlarmKitAlarms.schedule(.fixed(date)), .fixed(date))
        XCTAssertEqual(AlarmKitAlarms.schedule(.relative(hour: 7, minute: 5, weekdays: [1, 2, 7])),
                       .relative(.init(time: .init(hour: 7, minute: 5), repeats: .weekly([.sunday, .monday, .saturday]))))
        XCTAssertEqual(AlarmKitAlarms.schedule(.relative(hour: 6, minute: 0, weekdays: [])),
                       .relative(.init(time: .init(hour: 6, minute: 0), repeats: .never)))
        XCTAssertNil(AlarmKitAlarms.weekday(0))
        XCTAssertNil(AlarmKitAlarms.weekday(8))
    }
    #endif
}
