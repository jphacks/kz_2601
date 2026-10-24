import XCTest
@testable import MetamonDSL

/// `docs/dsl/examples/gallery-web.json`(webView + LINE/Slack/Discord 共有 + カスタムスキーム)が Swift 側でもフルにデコードできること、
/// および `ShareTarget` / `ExternalURLPolicy` の純粋ロジック。
final class GalleryWebTests: XCTestCase {
    private func gallery() throws -> AppDSL { try ExamplesFixture.load("gallery-web") }

    func testWebViewAndPresetsDecode() throws {
        let dsl = try gallery()
        let types = Set(dsl.screens.flatMap(\.allComponents).map(\.type))
        XCTAssertTrue(types.contains(.webView))
        XCTAssertFalse(types.contains(.unknown))
        XCTAssertFalse(ComponentType.webView.isContainer)

        let news = try XCTUnwrap(dsl.screen("news"))
        let web = try XCTUnwrap(news.components.first { $0.type == .webView })
        XCTAssertEqual(web.url, "https://jphacks.com")
        XCTAssertEqual(web.title, "JPHACKS")
        XCTAssertEqual(web.gridArea, GridArea("A1:F12"))

        let share = try XCTUnwrap(dsl.screen("share"))
        let presets = share.components.filter { $0.type == .shareButton }.map { ShareTarget($0.shareTo) }
        XCTAssertEqual(presets, [.line, .slack, .discord, .any])
        let schemes = share.components.filter { $0.type == .openUrlButton }.compactMap { ExternalURLPolicy.openable($0.url)?.scheme }
        XCTAssertEqual(schemes, ["maps", "tel", "mailto", "line"])

        let form = try XCTUnwrap(dsl.screen("clip_form"))
        let clip = try XCTUnwrap(form.components.first { $0.type == .shareButton })
        XCTAssertEqual(clip.shareTo, "line")
        XCTAssertEqual(clip.field, "comment")
        for s in dsl.screens { XCTAssertTrue(s.overlappingAreas.isEmpty, "\(s.id): area が重なっている") }
    }

    func testShareTargetLenientAndDefaults() {
        XCTAssertEqual(ShareTarget(nil), .any)
        XCTAssertEqual(ShareTarget("telegram"), .any)
        XCTAssertEqual(ShareTarget("line"), .line)
        XCTAssertEqual(ShareTarget.line.defaultLabel, "LINE で送る")
    }

    func testLineShareURLEncodesText() throws {
        let url = try XCTUnwrap(ShareTarget.lineShareURL(text: "今日の記事 https://jphacks.com?a=1&b=2"))
        XCTAssertEqual(url.scheme, "https")
        XCTAssertEqual(url.host, "line.me")
        XCTAssertEqual(url.path, "/R/share")
        let q = try XCTUnwrap(URLComponents(url: url, resolvingAgainstBaseURL: false)?.queryItems)
        XCTAssertEqual(q.count, 1)
        XCTAssertEqual(q.first?.name, "text")
        XCTAssertEqual(q.first?.value, "今日の記事 https://jphacks.com?a=1&b=2")
        XCTAssertFalse(url.absoluteString.contains("&b="), "& は text の一部としてエンコードされる")
    }

    func testChannelFormattingIsPlain() {
        let md = "**今日の記事**\n\n\n- 一言: `test`\n# 見出し\n-5度でした"
        let plain = ShareTarget.slack.format(md)
        XCTAssertEqual(plain, "💬 今日の記事\n\n一言: test\n見出し\n-5度でした")
        XCTAssertEqual(ShareTarget.discord.format(md), plain)
        XCTAssertEqual(ShareTarget.line.format(md), md, "LINE はそのまま")
        XCTAssertEqual(ShareTarget.any.format(md), md)
    }

    func testExternalURLPolicy() {
        for ok in ["https://a.com", "line://nv/chat", "slack://open", "discord://", "tel:0761234567", "mailto:a@b.c", "maps:?q=x", "comgooglemaps://?q=x", "HTTPS://A.COM"] {
            XCTAssertNotNil(ExternalURLPolicy.openable(ok), ok)
        }
        for ng in ["http://a.com", "ftp://a.com", "javascript:alert(1)", "file:///etc/passwd", "telegram://x", nil] {
            XCTAssertNil(ExternalURLPolicy.openable(ng), ng ?? "nil")
        }
        XCTAssertNotNil(ExternalURLPolicy.webViewURL("https://jphacks.com/path?x=1"))
        XCTAssertNil(ExternalURLPolicy.webViewURL("http://jphacks.com"))
        XCTAssertNil(ExternalURLPolicy.webViewURL("line://nv/chat"))
        XCTAssertNil(ExternalURLPolicy.webViewURL("https://"))
    }
}
