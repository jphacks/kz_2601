#if !APP_STORE_REVIEW
import XCTest
@testable import Metamon

/// #975: 外から届いた URL の読み方(Mac と共有する Shared/Runtime/IncomingLink)。iOS の ShareURL.token と同じ結果になる。
final class IncomingLinkTests: XCTestCase {
    func testShareSchemeBecomesShareLink() {
        XCTAssertEqual(IncomingLink(url: URL(string: "tailorit://import/tok_1")!), .share(token: "tok_1"))
        XCTAssertEqual(IncomingLink(url: URL(string: "TailorIt://Import/tok_1")!), .share(token: "tok_1"))
    }

    func testUnknownURLsAreIgnored() {
        for url in [
            "tailorit://import/",
            "tailorit://other/tok_1",
            "tailorit://import/a%20b",
            "otherapp://import/tok_1",
            "https://example.com/s/tok_1",
            "https://tailor-it.app/terms",
        ] {
            XCTAssertNil(IncomingLink(url: URL(string: url)!), url)
        }
    }

    /// Universal Links は Web 閲覧の activity だけ。ほかの種類の activity は URL があっても見ない
    func testActivityMustBeWebBrowsing() {
        let other = NSUserActivity(activityType: "jp.kz2601.metamon.other")
        other.webpageURL = URL(string: "https://tailor-it.app/s/tok_1")!
        XCTAssertNil(IncomingLink(activity: other))

        let unknown = NSUserActivity(activityType: NSUserActivityTypeBrowsingWeb)
        unknown.webpageURL = URL(string: "https://tailor-it.app/terms")!
        XCTAssertNil(IncomingLink(activity: unknown))

        XCTAssertNil(IncomingLink(activity: NSUserActivity(activityType: NSUserActivityTypeBrowsingWeb)))
    }
}
#endif
