import XCTest
@testable import MetamonServices

final class SharingTests: XCTestCase {
    func testShareURLToken() {
        XCTAssertEqual(ShareURL.token(from: URL(string: "tailorit://import/AbC_12-x")!), "AbC_12-x")
        XCTAssertEqual(ShareURL.token(from: URL(string: "TailorIt://IMPORT/tok")!), "tok")
        XCTAssertNil(ShareURL.token(from: URL(string: "tailorit://import/")!))
        XCTAssertNil(ShareURL.token(from: URL(string: "tailorit://open/tok")!))
        XCTAssertNil(ShareURL.token(from: URL(string: "https://example.com/import/tok")!))
        XCTAssertNil(ShareURL.token(from: URL(string: "tailorit://import/a%2Fb")!))
    }

    func testDecodeInstalledAppAndListing() throws {
        let installed = try JSONDecoder().decode(InstalledApp.self, from: Data(#"{"id":"abc","name":"家計簿","currentVersion":2,"joined":true,"created":true,"dsl":{}}"#.utf8))
        XCTAssertEqual(installed, InstalledApp(appId: "abc", name: "家計簿", joined: true))
        let listing = try JSONDecoder().decode(PublicAppListing.self, from: Data(#"{"id":"x","name":"散歩メモ","description":"d","tags":["散歩"],"authorName":"a","components":["list"],"dslVersion":3,"installs":4,"mine":false,"createdAt":"2026-10-24T00:00:00Z","updatedAt":"2026-10-24T00:00:00Z"}"#.utf8))
        XCTAssertEqual(listing.installs, 4)
        XCTAssertEqual(listing.components, ["list"])
    }
}
