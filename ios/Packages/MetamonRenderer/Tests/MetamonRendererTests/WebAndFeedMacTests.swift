#if !APP_STORE_REVIEW
import XCTest
import SwiftUI
import Observation
import MetamonDSL
@testable import MetamonRenderer
#if os(macOS)
import AppKit
#endif

/// #961: webView と feed を Mac でも iOS と同じ判定で出す。判定は両方の OS で回し、描画は Mac だけで確かめる。
@MainActor
final class WebAndFeedMacTests: XCTestCase {
    // MARK: - webView の URL とリンク

    func testWebViewShowsOnlyHttps() {
        XCTAssertNotNil(ExternalURLPolicy.webViewURL("https://example.com/a"))
        XCTAssertNil(ExternalURLPolicy.webViewURL("http://example.com/"))
        XCTAssertNil(ExternalURLPolicy.webViewURL("file:///etc/hosts"))
        XCTAssertNil(ExternalURLPolicy.webViewURL("javascript:alert(1)"))
        XCTAssertNil(ExternalURLPolicy.webViewURL("https:///nohost"))
        XCTAssertNil(ExternalURLPolicy.webViewURL(nil))
    }

    func testLinkPolicyInsidePage() throws {
        let https = try XCTUnwrap(URL(string: "https://example.com/next"))
        XCTAssertEqual(WebLinkPolicy.decide(https, opensNewWindow: false), .allow)
        XCTAssertEqual(WebLinkPolicy.decide(URL(string: "about:blank"), opensNewWindow: false), .allow)
        let tel = try XCTUnwrap(URL(string: "tel:0120000000"))
        XCTAssertEqual(WebLinkPolicy.decide(tel, opensNewWindow: false), .openExternally(tel), "allowlist のスキームは OS に渡す")
        let mail = try XCTUnwrap(URL(string: "mailto:a@example.com"))
        XCTAssertEqual(WebLinkPolicy.decide(mail, opensNewWindow: false), .openExternally(mail))
        XCTAssertEqual(WebLinkPolicy.decide(URL(string: "file:///etc/hosts"), opensNewWindow: false), .ignore)
        XCTAssertEqual(WebLinkPolicy.decide(URL(string: "javascript:alert(1)"), opensNewWindow: false), .ignore)
        XCTAssertEqual(WebLinkPolicy.decide(nil, opensNewWindow: false), .ignore)
    }

    /// 新しいウインドウで開くリンク: Mac は既定のブラウザに渡す、iOS は今まで通り(WKWebView に任せる)。
    func testNewWindowLinks() throws {
        let https = try XCTUnwrap(URL(string: "https://example.com/blank"))
        XCTAssertEqual(WebLinkPolicy.decide(https, opensNewWindow: true, externalNewWindows: true), .openExternally(https))
        XCTAssertEqual(WebLinkPolicy.decide(https, opensNewWindow: true, externalNewWindows: false), .allow)
        XCTAssertEqual(WebLinkPolicy.decide(https, opensNewWindow: true, linkActivated: false, externalNewWindows: true), .ignore,
                       "スクリプトが開くウインドウはブラウザに渡さない")
        let http = try XCTUnwrap(URL(string: "http://example.com/blank"))
        XCTAssertEqual(WebLinkPolicy.decide(http, opensNewWindow: true, externalNewWindows: true), .allow, "http は外に渡さない")
        #if os(macOS)
        XCTAssertTrue(WebLinkPolicy.isMac)
        #else
        XCTAssertFalse(WebLinkPolicy.isMac)
        #endif
    }

    func testWebViewIsRegistered() {
        XCTAssertNotNil(ComponentRegistry.standard.builder(for: ComponentType.webView.rawValue))
        XCTAssertNotNil(ComponentRegistry.standard.builder(for: ComponentType.feed.rawValue))
    }

    // MARK: - feed

    func testFeedEmptyState() {
        XCTAssertEqual(FeedLeaf.emptyState(loaded: false, failed: false), .loading)
        XCTAssertEqual(FeedLeaf.emptyState(loaded: false, failed: true), .unavailable)
        XCTAssertEqual(FeedLeaf.emptyState(loaded: true, failed: false), .empty)
    }

    /// FeedHost を持たないホスト(今の Mac・モック)では feeds が nil →「読み込めませんでした」と再試行。
    func testRuntimeWithoutFeedHost() throws {
        let rt = FeedTestRuntime(dsl: try dsl())
        XCTAssertNil(rt.feeds)
        XCTAssertNotNil(FeedTestHostRuntime(dsl: try dsl()).feeds)
    }

    #if os(macOS)
    /// Mac のウインドウに置くと、feed がホストに取りに行く(.task が動く)。
    func testFeedLoadsThroughHostOnMac() throws {
        let rt = FeedTestHostRuntime(dsl: try dsl())
        let c = try XCTUnwrap(rt.dsl.screens.first?.components.first { $0.type == .feed })
        let host = host(FeedLeaf(c: c).environment(\.dslRuntime, rt))
        defer { host.close() }
        let loaded = expectation(description: "feed")
        Task { @MainActor in
            for _ in 0..<100 where rt.calls.isEmpty { try? await Task.sleep(for: .milliseconds(20)) }
            loaded.fulfill()
        }
        wait(for: [loaded], timeout: 3)
        XCTAssertEqual(rt.calls, ["weather"])
    }

    /// webView と feed(ホスト無し)の画面が Mac で描ける(落ちない・高さがある)。
    func testWebViewAndFeedRenderOnMac() throws {
        let d = try dsl()
        let rt = FeedTestRuntime(dsl: d)
        for c in try XCTUnwrap(d.screens.first?.components) {
            let view = ComponentRegistry.standard.builder(for: c.rawType).map {
                $0(c, RenderContext(route: Route(screen: "home", stack: "main")))
            }
            let window = host(try XCTUnwrap(view).environment(\.dslRuntime, rt).frame(width: 480, height: 360))
            defer { window.close() }
            XCTAssertGreaterThan(window.contentView?.fittingSize.height ?? 0, 0, c.rawType)
        }
    }

    private func host<V: View>(_ view: V) -> NSWindow {
        let window = NSWindow(contentRect: CGRect(x: 0, y: 0, width: 480, height: 360), styleMask: [.titled], backing: .buffered, defer: false)
        window.isReleasedWhenClosed = false
        window.contentView = NSHostingView(rootView: view)
        window.orderFrontRegardless()
        window.contentView?.layoutSubtreeIfNeeded()
        return window
    }
    #endif

    private func dsl() throws -> AppDSL {
        try JSONDecoder().decode(AppDSL.self, from: Data(#"""
        {"schemaVersion":1,"app":{"name":"Web","voiceEntry":"none"},"initialScreen":"home","entities":[],
         "screens":[{"id":"home","title":"Web","components":[
           {"type":"webView","id":"w","url":"https://example.com/"},
           {"type":"feed","id":"wx","title":"天気","provider":"weather","region":"170000"}]}]}
        """#.utf8))
    }
}

@MainActor @Observable
private final class FeedTestRuntime: DSLRuntime {
    var dsl: AppDSL
    var paths: [String: [Route]] = [:]
    var selectedTab = ""
    var loading: Set<String> = []
    init(dsl: AppDSL) { self.dsl = dsl }
    func records(for entity: String) -> [Record] { [] }
    func load(entities: Set<String>, force: Bool) async {}
    func run(_ action: Action, _ ctx: ActionContext) async {}
    func close(_ route: Route) {}
    func pathBinding(_ stack: String) -> Binding<[Route]> { .constant([]) }
}

/// 外へは出ない。決まった 1 行を返す。
@MainActor @Observable
private final class FeedTestHostRuntime: DSLRuntime, FeedHost {
    var dsl: AppDSL
    var paths: [String: [Route]] = [:]
    var selectedTab = ""
    var loading: Set<String> = []
    var calls: [String] = []
    init(dsl: AppDSL) { self.dsl = dsl }
    func records(for entity: String) -> [Record] { [] }
    func load(entities: Set<String>, force: Bool) async {}
    func run(_ action: Action, _ ctx: ActionContext) async {}
    func close(_ route: Route) {}
    func pathBinding(_ stack: String) -> Binding<[Route]> { .constant([]) }
    func feed(_ query: FeedQuery, force: Bool) async throws -> FeedResult {
        calls.append(query.provider)
        return FeedResult(provider: query.provider, title: "天気", items: [FeedItem(date: "2026-11-07", title: "晴れ", max: 18, min: 9)],
                          source: FeedSource(name: "気象庁ホームページ", url: "https://www.jma.go.jp/", license: "公共データ利用規約(第1.0版)"))
    }
}
#endif
