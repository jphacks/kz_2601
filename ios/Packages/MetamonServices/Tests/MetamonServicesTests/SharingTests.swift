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
        XCTAssertNil(ShareURL.token(from: URL(string: "tailorit://import/..")!))
        XCTAssertNil(ShareURL.token(from: URL(string: "tailorit://import/.")!))
        XCTAssertNil(ShareURL.token(from: URL(string: "tailorit://import/%2e%2e")!))
        XCTAssertNil(ShareURL.token(from: URL(string: "tailorit://import/%C3%A9")!))
        XCTAssertNil(ShareURL.token(from: URL(string: "tailorit://import/" + String(repeating: "a", count: 129))!))
        XCTAssertEqual(ShareURL.token(from: URL(string: "tailorit://import/a.b")!), "a.b")
        XCTAssertEqual(ShareURL.token(from: URL(string: "tailorit://import/" + String(repeating: "a", count: 128))!)?.count, 128)
    }

    func testHTTPClientRejectsBadTokenBeforeSending() async {
        let client = HTTPSharingClient(baseURL: URL(string: "https://example.invalid/")!, token: NoAuthTokenProvider())
        for token in ["..", ".", "a/b"] {
            do {
                _ = try await client.previewShare(token: token)
                XCTFail("previewShare(\(token)) should throw")
            } catch {
                assertInvalidURL(error)
            }
            do {
                _ = try await client.importShare(token: token, displayName: "")
                XCTFail("importShare(\(token)) should throw")
            } catch {
                assertInvalidURL(error)
            }
            do {
                try await client.revokeShare(token: token)
                XCTFail("revokeShare(\(token)) should throw")
            } catch {
                assertInvalidURL(error)
            }
        }
    }

    /// #331: Universal Links `https://tailor-it.app/s/<token>` も同じトークンにする
    func testShareURLTokenFromUniversalLink() {
        XCTAssertEqual(ShareURL.token(from: URL(string: "https://tailor-it.app/s/AbC_12-x")!), "AbC_12-x")
        XCTAssertEqual(ShareURL.token(from: URL(string: "https://Tailor-It.app/s/tok")!), "tok")
        XCTAssertEqual(ShareURL.token(from: URL(string: "https://tailor-it.app/s/tok/")!), "tok")
        XCTAssertEqual(ShareURL.token(from: URL(string: "https://tailor-it.app/s/tok?utm_source=x")!), "tok")
        for bad in [
            "https://tailor-it.app/s/",
            "https://tailor-it.app/s",
            "https://tailor-it.app/s/tok/extra",
            "https://tailor-it.app/terms",
            "https://tailor-it.app/x/tok",
            "https://tailor-it.app:8443/s/tok",
            "http://tailor-it.app/s/tok",
            "https://www.tailor-it.app/s/tok",
            "https://tailor-it.app.evil.example/s/tok",
            "https://evil.example/s/tok",
            "https://tailor-it.app/s/a%2Fb",
            "https://tailor-it.app/s/a%20b",
        ] {
            XCTAssertNil(ShareURL.token(from: URL(string: bad)!), bad)
        }
    }

    func testDecodeInstalledAppAndListing() throws {
        let installed = try JSONDecoder().decode(InstalledApp.self, from: Data(#"{"id":"abc","name":"家計簿","currentVersion":2,"joined":true,"created":true,"dsl":{}}"#.utf8))
        XCTAssertEqual(installed, InstalledApp(appId: "abc", name: "家計簿", joined: true))
        let listing = try JSONDecoder().decode(PublicAppListing.self, from: Data(#"{"id":"x","name":"散歩メモ","description":"d","tags":["散歩"],"authorName":"a","components":["list"],"dslVersion":3,"installs":4,"mine":false,"createdAt":"2026-10-24T00:00:00Z","updatedAt":"2026-10-24T00:00:00Z"}"#.utf8))
        XCTAssertEqual(listing.installs, 4)
        XCTAssertEqual(listing.components, ["list"])
    }

    private func assertInvalidURL(_ error: Error, file: StaticString = #filePath, line: UInt = #line) {
        guard case .invalidURL? = error as? APIError else {
            return XCTFail("expected invalidURL, got \(error)", file: file, line: line)
        }
    }
}
