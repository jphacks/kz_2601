import XCTest
import SwiftUI
import MetamonDSL
@testable import MetamonRenderer
#if os(macOS)
import AppKit
#endif

/// Container 7 種(vstack・hstack・section・card・disclosure・carousel・grid)が子を描くか(#940)と、
/// Mac の carousel の送り・折りたたみの読み上げ(#937)。iOS でも同じテストが通る(描画の木は共通)。
@MainActor
final class ContainerRenderTests: XCTestCase {
    private final class Seen: @unchecked Sendable { var ids: [String] = [] }

    private func component(_ json: String) throws -> Component {
        try JSONDecoder().decode(Component.self, from: Data(json.utf8))
    }

    private func container(_ type: String, extra: String = "") throws -> Component {
        try component(#"{"id":"c","type":""# + type + #"","title":"見出し""# + extra + #","children":[{"id":"a","type":"text","text":"A"},{"id":"b","type":"text","text":"B"},{"id":"d","type":"text","text":"D"}]}"#)
    }

    /// 描いた葉の text の id(builder が呼ばれた順)。
    private func drawnLeaves(_ c: Component, width: CGFloat = 900) -> Set<String> {
        let seen = Seen()
        let registry = ComponentRegistry.standard.registering(.text) { leaf, _ in
            let _ = { seen.ids.append(leaf.id) }()
            Text(leaf.text ?? "")
        }
        let view = DSLView(component: c, ctx: RenderContext(route: Route(screen: "home")))
            .environment(\.componentRegistry, registry)
            .frame(width: width, height: 400)
        #if os(macOS)
        // AppKit の実際のレイアウト(スクロールの中の Lazy な並びも測る)
        let host = NSHostingView(rootView: view)
        host.frame = CGRect(x: 0, y: 0, width: width, height: 400)
        host.layoutSubtreeIfNeeded()
        #endif
        _ = ImageRenderer(content: view).platformImage
        return Set(seen.ids)
    }

    func testStacksSectionCardGridDrawAllChildren() throws {
        for type in ["vstack", "hstack", "section", "card", "grid"] {
            XCTAssertEqual(drawnLeaves(try container(type)), ["a", "b", "d"], type)
        }
        XCTAssertEqual(drawnLeaves(try container("grid", extra: #","columns":4"#)), ["a", "b", "d"], "grid 4 列")
    }

    func testDisclosureDrawsChildrenOnlyWhenExpanded() throws {
        XCTAssertEqual(drawnLeaves(try container("disclosure", extra: #","expanded":true"#)), ["a", "b", "d"])
        XCTAssertEqual(drawnLeaves(try container("disclosure")), [], "閉じている間は中身を描かない")
    }

    func testCarouselDrawsVisibleChildren() throws {
        // 幅 70% のカードなので、900pt の窓には少なくとも先頭の 1 枚が入る
        let drawn = drawnLeaves(try container("carousel", extra: #","itemWidth":"small""#))
        XCTAssertTrue(drawn.contains("a"), "先頭のカードを描く: \(drawn)")
    }

    func testCarouselPagingStopsAtEnds() {
        XCTAssertEqual(CarouselPaging.step(0, by: -1, count: 3), 0)
        XCTAssertEqual(CarouselPaging.step(0, by: 1, count: 3), 1)
        XCTAssertEqual(CarouselPaging.step(2, by: 1, count: 3), 2)
        XCTAssertEqual(CarouselPaging.step(5, by: 0, count: 3), 2, "子が減った時は最後のページ")
        XCTAssertEqual(CarouselPaging.step(0, by: 1, count: 0), 0)
        XCTAssertFalse(CarouselPaging.canGoBack(0, count: 3))
        XCTAssertTrue(CarouselPaging.canGoForward(0, count: 3))
        XCTAssertFalse(CarouselPaging.canGoForward(2, count: 3))
        XCTAssertFalse(CarouselPaging.showsControls(count: 1))
        XCTAssertTrue(CarouselPaging.showsControls(count: 2))
        XCTAssertEqual(CarouselPaging.positionText(1, count: 5), "2 / 5")
        XCTAssertEqual(CarouselPaging.positionText(0, count: 0), "")
    }

    func testDisclosureAccessibilityCopyIsEnglish() {
        let saved = LanguagePreference.shared.setting
        defer { LanguagePreference.shared.setting = saved }
        LanguagePreference.shared.setting = .en
        XCTAssertEqual(DisclosureAccessibility.value(expanded: true), "Expanded")
        XCTAssertEqual(DisclosureAccessibility.value(expanded: false), "Collapsed")
        XCTAssertEqual(DisclosureAccessibility.hint(expanded: false), "Expands")
        XCTAssertEqual(L("前へ"), "Previous")
        XCTAssertEqual(L("次へ"), "Next")
    }
}
