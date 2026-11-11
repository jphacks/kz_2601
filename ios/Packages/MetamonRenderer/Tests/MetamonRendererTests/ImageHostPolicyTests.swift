import XCTest
@testable import MetamonRenderer

/// 画像を読む先は公開のホストの https だけ(Android の ImageHostsTest と同じ例)。
final class ImageHostPolicyTests: XCTestCase {
    func testPublicHttpsIsLoadable() {
        XCTAssertNotNil(ImageHostPolicy.loadableURL("https://images.example.com/a.jpg"))
        XCTAssertNotNil(ImageHostPolicy.loadableURL("  https://example.com/a.jpg "))
        XCTAssertNotNil(ImageHostPolicy.loadableURL("HTTPS://example.com/a.jpg"))
        XCTAssertNotNil(ImageHostPolicy.loadableURL("https://example.com./a.jpg"))
        XCTAssertNotNil(ImageHostPolicy.loadableURL("https://localhost.example.com/a.jpg"))
        XCTAssertNotNil(ImageHostPolicy.loadableURL("https://firebasestorage.googleapis.com/v0/b/x/o/a.jpg?alt=media"))
    }

    func testLocalAndLiteralHostsAreNotLoadable() {
        let blocked = [
            "http://example.com/a.jpg",
            "https:///a.jpg",
            "https://user:pass@example.com/a.jpg",
            "file:///tmp/a.jpg",
            "https://localhost/a.jpg",
            "https://LOCALHOST:8443/a.jpg",
            "https://foo.localhost/a.jpg",
            "https://printer.local/a.jpg",
            "https://svc.internal/a.jpg",
            "https://nas.home.arpa/a.jpg",
            "https://router/a.jpg",
            "https://127.0.0.1/a.jpg",
            "https://10.0.0.5/a.jpg",
            "https://192.168.1.1/a.jpg",
            "https://8.8.8.8/a.jpg",
            "https://2130706433/a.jpg",
            "https://0x7f000001/a.jpg",
            "https://0x7f.0.0.1/a.jpg",
            "https://127.1/a.jpg",
            "https://[::1]/a.jpg",
            "https://[2606:4700::1111]/a.jpg",
            "https://example.com/" + String(repeating: "a", count: ImageHostPolicy.maxURLLength),
            "",
        ]
        for s in blocked { XCTAssertNil(ImageHostPolicy.loadableURL(s), s) }
        XCTAssertNil(ImageHostPolicy.loadableURL(nil))
    }
}
