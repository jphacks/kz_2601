import XCTest
@testable import MetamonDSL

/// `openUrlButton.app` と `shareButton.shareTo` x / mail の URL 作り(エンコード・形に合わない時のトップ)。
final class OpenAppTests: XCTestCase {
    private var savedLanguage: String?
    override func setUp() { savedLanguage = setAppLanguage("ja") } // 文言は日本語で確かめる(シミュレータの言語に依らない)
    override func tearDown() { setAppLanguage(savedLanguage) }

    func testEnglishLabels() {
        setAppLanguage("en")
        XCTAssertEqual(OpenApp.paypay.defaultLabel, "Open PayPay")
        XCTAssertEqual(OpenApp.maps.defaultLabel, "Open in Maps")
        XCTAssertEqual(ShareTarget.x.defaultLabel, "Post to X")
        XCTAssertEqual(ShareTarget.mail.defaultLabel, "Send by email")
    }

    private func strings(_ app: OpenApp, _ to: String?) -> [String] { app.urls(to: to).map(\.absoluteString) }

    func testLenientDecode() throws {
        XCTAssertEqual(OpenApp("paypay"), .paypay)
        XCTAssertNil(OpenApp("telegram"))
        XCTAssertNil(OpenApp(nil))
        let json = #"{"type":"openUrlButton","app":"phone","to":"{{tel}}"}"#
        let c = try JSONDecoder().decode(Component.self, from: Data(json.utf8))
        XCTAssertEqual(c.app, "phone")
        XCTAssertEqual(c.to, "{{tel}}")
    }

    func testHomeWithoutTo() {
        XCTAssertEqual(strings(.paypay, nil), ["https://paypay.ne.jp/"])
        XCTAssertEqual(strings(.line, "  "), ["https://line.me/R/nv/chat"])
        XCTAssertEqual(strings(.slack, nil), ["slack://open", "https://slack.com/"], "アプリが無ければ Web")
        XCTAssertEqual(strings(.discord, nil), ["https://discord.com/app"])
        XCTAssertEqual(strings(.maps, nil), ["https://maps.apple.com/"])
        XCTAssertEqual(strings(.mail, nil), ["mailto:"])
        XCTAssertEqual(strings(.phone, nil), [], "番号が無ければ開かない")
    }

    func testServiceLinks() {
        XCTAssertEqual(strings(.paypay, "https://qr.paypay.ne.jp/p2p01_abc"), ["https://qr.paypay.ne.jp/p2p01_abc"])
        XCTAssertEqual(strings(.line, "https://line.me/R/ti/p/%40shop"), ["https://line.me/R/ti/p/%40shop"])
        XCTAssertEqual(strings(.slack, "https://team.slack.com/archives/C1"), ["https://team.slack.com/archives/C1"])
        XCTAssertEqual(strings(.discord, "https://discord.gg/abc"), ["https://discord.gg/abc"])
    }

    func testForeignLinksFallBackToHome() {
        for to in ["https://paypay.ne.jp.evil.example/", "http://qr.paypay.ne.jp/x", "https://user@paypay.ne.jp/", "javascript:alert(1)", "paypay://pay"] {
            XCTAssertEqual(strings(.paypay, to), ["https://paypay.ne.jp/"], to)
        }
        XCTAssertEqual(strings(.slack, "https://slack.com.evil.example/"), ["slack://open", "https://slack.com/"])
    }

    func testUsernames() {
        XCTAssertEqual(strings(.x, "@jphacks"), ["https://x.com/jphacks"])
        XCTAssertEqual(strings(.x, "https://x.com/jphacks"), ["https://x.com/jphacks"])
        XCTAssertEqual(strings(.x, "a/b?c"), ["https://x.com/"], "パスを足させない")
        XCTAssertEqual(strings(.instagram, "some.user_1"), ["https://www.instagram.com/some.user_1/"])
        XCTAssertEqual(strings(.instagram, "日本語"), ["https://www.instagram.com/"])
    }

    func testMapsQueryIsEncoded() throws {
        let url = try XCTUnwrap(OpenApp.maps.urls(to: "金沢駅 東口&b=1").first)
        XCTAssertEqual(url.host, "maps.apple.com")
        let q = try XCTUnwrap(URLComponents(url: url, resolvingAgainstBaseURL: false)?.queryItems)
        XCTAssertEqual(q.map(\.name), ["q"])
        XCTAssertEqual(q.first?.value, "金沢駅 東口&b=1")
    }

    func testMailAndPhone() {
        XCTAssertEqual(strings(.mail, "a.b@example.com"), ["mailto:a.b@example.com"])
        XCTAssertEqual(strings(.mail, "a@b.c?cc=x@y.z"), ["mailto:"], "クエリを足させない")
        XCTAssertEqual(strings(.phone, "076-123-4567"), ["tel:0761234567"])
        XCTAssertEqual(strings(.phone, "+81 (76) 123 4567"), ["tel:+81761234567"])
        XCTAssertEqual(strings(.phone, "０７６１２３"), ["tel:076123"], "全角数字も")
        XCTAssertEqual(strings(.phone, "call me"), [])
        XCTAssertEqual(strings(.phone, "1;x"), [])
    }

    func testDefaults() {
        XCTAssertEqual(OpenApp.paypay.defaultLabel, "PayPay を開く")
        for app in OpenApp.allCases {
            XCTAssertFalse(app.defaultLabel.isEmpty)
            XCTAssertFalse(app.defaultIcon.isEmpty)
        }
    }

    // MARK: - shareTo x / mail

    func testShareTargetNewPresets() {
        XCTAssertEqual(ShareTarget("x"), .x)
        XCTAssertEqual(ShareTarget("mail"), .mail)
        XCTAssertEqual(ShareTarget.x.defaultLabel, "X に投稿")
        XCTAssertNil(ShareTarget.any.directURL(text: "a"))
        XCTAssertNil(ShareTarget.slack.directURL(text: "a"), "Slack は共有シート")
        XCTAssertEqual(ShareTarget.line.directURL(text: "a")?.host, "line.me")
    }

    func testXPostURLEncodesText() throws {
        let url = try XCTUnwrap(ShareTarget.x.directURL(text: "ランチ 1200円 #割り勘 a&b=c+d"))
        XCTAssertEqual(url.host, "x.com")
        XCTAssertEqual(url.path, "/intent/tweet")
        let q = try XCTUnwrap(URLComponents(url: url, resolvingAgainstBaseURL: false)?.queryItems)
        XCTAssertEqual(q.map(\.name), ["text"])
        XCTAssertEqual(q.first?.value, "ランチ 1200円 #割り勘 a&b=c+d")
        XCTAssertFalse(url.absoluteString.contains("#"), "# はフラグメントにしない")
    }

    func testMailBody() throws {
        let url = try XCTUnwrap(ShareTarget.mail.directURL(text: "合計 3000円\n1 人 1000円"))
        XCTAssertEqual(url.scheme, "mailto")
        XCTAssertEqual(url.absoluteString, "mailto:?body=%E5%90%88%E8%A8%88%203000%E5%86%86%0A1%20%E4%BA%BA%201000%E5%86%86")
        XCTAssertEqual(ShareTarget.mailURL(to: "a@b.jp", body: "x y")?.absoluteString, "mailto:a@b.jp?body=x%20y")
    }

    /// 文面は line と同じくそのまま(Markdown を落とすのは slack / discord だけ)。
    func testFormatKeepsText() {
        XCTAssertEqual(ShareTarget.x.format("**a**"), "**a**")
        XCTAssertEqual(ShareTarget.mail.format("**a**"), "**a**")
    }
}
