import XCTest
import SwiftUI
import MetamonDSL
@testable import MetamonRenderer

/// #536: レジストリに無い部品は、リリース版でも枠と理由を出す(何も描かない EmptyView にしない)。
final class UnavailableComponentTests: XCTestCase {
    private var saved: AppLanguage = .system

    override func setUp() { saved = LanguagePreference.shared.setting }
    override func tearDown() { LanguagePreference.shared.setting = saved }

    /// 新しい版の部品(hologram)・この版で外した扱いの部品(healthStat)・普通の部品(text)を並べた DSL。
    private func components() throws -> [Component] {
        try JSONDecoder().decode([Component].self, from: Data(#"""
        [
          {"id":"future","type":"hologram","area":"A1:B2"},
          {"id":"excluded","type":"healthStat","metric":"steps","area":"C1:D2"},
          {"id":"plain","type":"text","text":"こんにちは"}
        ]
        """#.utf8))
    }

    /// `text` だけ描けるレジストリ(審査用の版で部品を外した状態の代わり)。
    private var limitedRegistry: ComponentRegistry {
        var r = ComponentRegistry()
        r.register(.text) { c, _ in TextLeaf(c: c) }
        return r
    }

    @MainActor private func render<V: View>(_ view: V) -> UIImage? {
        let r = ImageRenderer(content: view.frame(width: 160))
        r.scale = 1
        return r.uiImage
    }

    func testReasonSplitsNewerTypeFromExcludedType() throws {
        let cs = try components()
        XCTAssertEqual(cs[0].type, .unknown)
        XCTAssertEqual(UnavailableComponent.Reason(cs[0]), .needsUpdate, "この版が知らない type = 新しい版の部品")
        XCTAssertEqual(UnavailableComponent.Reason(cs[1]), .unavailableHere, "知っている type がレジストリに無い = この版で外した部品")
        XCTAssertNil(limitedRegistry.builder(for: cs[1].rawType))
    }

    func testReviewExcludedComponentsShowUnavailableHere() throws {
        for type in ClientCapabilities.reviewExcludedComponents {
            let c = try JSONDecoder().decode(Component.self, from: Data(#"{"type":"\#(type.rawValue)"}"#.utf8))
            XCTAssertEqual(UnavailableComponent.Reason(c), .unavailableHere, type.rawValue)
            #if APP_STORE_REVIEW
            XCTAssertNil(ComponentRegistry.standard.builder(for: type.rawValue), "審査用の版では登録しない: \(type.rawValue)")
            #endif
        }
    }

    func testMessagesAreGenericAndLocalized() throws {
        LanguagePreference.shared.setting = .ja
        XCTAssertEqual(UnavailableComponent.Reason.needsUpdate.message, "アプリを更新すると表示できます")
        XCTAssertEqual(UnavailableComponent.Reason.unavailableHere.message, "この端末では使えません")

        LanguagePreference.shared.setting = .en
        XCTAssertEqual(UnavailableComponent.Reason.needsUpdate.message, "Update the app to show this")
        XCTAssertEqual(UnavailableComponent.Reason.unavailableHere.message, "Not available on this device")

        // 審査用の版で部品名(機能名)を見せない
        for reason in [UnavailableComponent.Reason.needsUpdate, .unavailableHere] {
            for type in ClientCapabilities.reviewExcludedComponents {
                XCTAssertFalse(reason.message.localizedCaseInsensitiveContains(type.rawValue))
            }
        }
    }

    @MainActor func testDSLViewDrawsPlaceholderForUnregisteredTypes() throws {
        let ctx = RenderContext(route: Route(screen: "home"))
        for c in try components().prefix(2) {
            let view = DSLView(component: c, ctx: ctx).environment(\.componentRegistry, limitedRegistry)
            let image = try XCTUnwrap(render(view), "\(c.rawType): 何も描かない(EmptyView)にしない")
            XCTAssertGreaterThanOrEqual(image.size.height, 44, c.rawType)
        }
    }
}
