import XCTest
import MetamonDSL
@testable import MetamonRenderer

/// #253: 公開(public)・共有(shared)の entity の一覧とフォームに出す文言。private は出さない。
final class PublicDataNoticeTests: XCTestCase {
    func testTextPerScope() {
        XCTAssertNotNil(PublicDataNotice.text(for: .public))
        XCTAssertNotNil(PublicDataNotice.text(for: .shared))
        XCTAssertNotEqual(PublicDataNotice.text(for: .public), PublicDataNotice.text(for: .shared), "shared は「参加者だけ」")
        XCTAssertNil(PublicDataNotice.text(for: .private))
    }
}
