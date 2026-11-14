import XCTest
import SwiftUI
import Observation
import MetamonDSL
@testable import MetamonRenderer
#if os(macOS)
import AppKit
#endif

/// #941: 表示だけの leaf 15 種が Mac でも iOS と同じ内容で描け、VoiceOver が読む名前を持つ。
/// 判定は両方の OS で回し、ウインドウに置く確認は Mac だけ。キー操作と記号の言い換えは MetamonMacTests/MacDisplayLeavesTests。
/// 元は #2016(2410as)のテスト。
@MainActor
final class DisplayLeafMacTests: XCTestCase {
    static let displayTypes = ["text", "label", "markdown", "image", "icon", "avatar", "badge", "divider",
                               "spacer", "emptyState", "link", "timeline", "rating", "progressBar", "gauge"]

    // MARK: - 15 種が描ける

    func testAllDisplayLeavesAreRegistered() {
        for type in Self.displayTypes {
            XCTAssertNotNil(ComponentRegistry.standard.builder(for: type), type)
        }
    }

    func testEveryDisplayLeafRenders() throws {
        let fixture = try Fixture()
        XCTAssertEqual(fixture.components.map(\.rawType), Self.displayTypes, "15 種をそろえて確かめる")
        for c in fixture.components {
            let view = try XCTUnwrap(ComponentRegistry.standard.builder(for: c.rawType), c.rawType)(c, fixture.context)
            let image = ImageRenderer(content: view.environment(\.dslRuntime, fixture.runtime).frame(width: 360, height: 160)).platformImage
            XCTAssertNotNil(image, c.rawType)
        }
    }

    // MARK: - markdown のリンク

    /// 許されたスキームのリンクだけ生かす。外したリンクも文字は残す。
    func testMarkdownKeepsOnlyOpenableLinks() {
        let out = MarkdownLeaf.attributed("**強調** [公式](https://example.com/a) [罠](javascript:alert(1)) [相対](docs/a.md) [電話](tel:0120000000)")
        let links = out.runs.compactMap { run in run.link.map { (String(out[run.range].characters), $0.absoluteString) } }
        XCTAssertEqual(links.map(\.0), ["公式", "電話"], "https と allowlist のスキームだけリンクにする")
        XCTAssertEqual(links.first?.1, "https://example.com/a")
        let text = String(out.characters)
        XCTAssertTrue(text.contains("罠"))
        XCTAssertTrue(text.contains("相対"))
        XCTAssertFalse(text.contains("**"), "太字は解釈する")
        XCTAssertEqual(String(MarkdownLeaf.attributed("素の文です").characters), "素の文です")
    }

    // MARK: - timeline のコピー

    func testTimelineCopyTextIsOneLinePerRecord() throws {
        let fixture = try Fixture()
        let c = try XCTUnwrap(fixture.components.first { $0.rawType == "timeline" })
        let rows = TimelineLeaf.rows(c, rt: fixture.runtime)
        let text = TimelineLeaf.copyText(rows, c: c, rt: fixture.runtime)
        XCTAssertEqual(text.split(separator: "\n").count, rows.count)
        XCTAssertTrue(text.contains("朝ラン"))
    }

    // MARK: - VoiceOver が読む文言

    /// avatar は名前が無ければ「アバター」、rating は「評価」。日本語・英語の両方で訳がある。
    func testVoiceOverCopyIsLocalized() {
        let saved = LanguagePreference.shared.setting
        defer { LanguagePreference.shared.setting = saved }
        LanguagePreference.shared.setting = .ja
        XCTAssertEqual(L("アバター"), "アバター")
        XCTAssertEqual(L("評価"), "評価")
        LanguagePreference.shared.setting = .en
        XCTAssertEqual(L("アバター"), "Avatar")
        XCTAssertEqual(L("評価"), "Rating")
    }

    #if os(macOS)

    // MARK: - Mac のウインドウで

    func testEveryDisplayLeafHasHeightInAMacWindow() throws {
        let fixture = try Fixture()
        for c in fixture.components {
            let view = try XCTUnwrap(ComponentRegistry.standard.builder(for: c.rawType), c.rawType)(c, fixture.context)
            let window = Self.host(view.environment(\.dslRuntime, fixture.runtime).frame(width: 360))
            defer { window.close() }
            XCTAssertGreaterThan(window.contentView?.fittingSize.height ?? 0, 0, c.rawType)
        }
    }

    private static func host<V: View>(_ view: V) -> NSWindow {
        let window = NSWindow(contentRect: CGRect(x: 0, y: 0, width: 360, height: 200), styleMask: [.titled], backing: .buffered, defer: false)
        window.isReleasedWhenClosed = false
        window.contentView = NSHostingView(rootView: view)
        window.orderFrontRegardless()
        window.contentView?.layoutSubtreeIfNeeded()
        return window
    }
    #endif
}

/// 15 種を並べた 1 画面と、その画面のフォーム・ランタイム。
@MainActor
private struct Fixture {
    let dsl: AppDSL
    let runtime: DisplayLeafRuntime
    let context: RenderContext

    var components: [Component] { dsl.screens.first?.components ?? [] }

    init() throws {
        dsl = try JSONDecoder().decode(AppDSL.self, from: Data(#"""
        {"schemaVersion":1,"app":{"name":"表示","voiceEntry":"none"},"initialScreen":"home",
         "entities":[{"id":"log","name":"記録","fields":[
           {"id":"when","name":"日","type":"date"},
           {"id":"title","name":"題","type":"string"},
           {"id":"photo","name":"写真","type":"photo"},
           {"id":"score","name":"評価","type":"integer"},
           {"id":"weight","name":"体重","type":"number","unit":"kg"}]}],
         "screens":[{"id":"home","title":"表示","entity":"log","components":[
           {"type":"text","id":"t","text":"期限は {{title}}","style":"headline","highlight":{"words":["期限"]}},
           {"type":"label","id":"lb","text":"今日のまとめ","icon":"zzz.not.a.real.symbol"},
           {"type":"markdown","id":"md","text":"**太字** と [公式](https://example.com/a)"},
           {"type":"image","id":"im","field":"photo","height":"small"},
           {"type":"icon","id":"ic","name":"heart.fill","size":"medium","color":"red"},
           {"type":"avatar","id":"av","name":"杏奈","size":"medium"},
           {"type":"badge","id":"bd","text":"進行中","color":"blue"},
           {"type":"divider","id":"dv"},
           {"type":"spacer","id":"sp","size":"medium"},
           {"type":"emptyState","id":"es","title":"まだありません","message":"右上の + で足せます","icon":"tray"},
           {"type":"link","id":"lk","text":"公式サイト","url":"https://example.com/"},
           {"type":"timeline","id":"tl","entity":"log","dateField":"when","titleField":"title"},
           {"type":"rating","id":"rt","field":"score","label":"評価","max":5},
           {"type":"progressBar","id":"pb","label":"進捗","value":0.42},
           {"type":"gauge","id":"gg","label":"体重","value":62.5,"min":40,"max":120}]}]}
        """#.utf8))
        let entity = try XCTUnwrap(dsl.entities.first)
        runtime = DisplayLeafRuntime(dsl: dsl, rows: [Record(id: "r1", data: ["when": "2026-11-05", "title": "朝ラン"])])
        let form = FormDraft(entity: entity)
        form.values["title"] = "明日"
        form.values["score"] = 3
        context = RenderContext(route: Route(screen: "home", stack: "main"), form: form)
    }
}

/// 外に出ない・決まった行を返すだけのランタイム。
@MainActor @Observable
private final class DisplayLeafRuntime: DSLRuntime {
    var dsl: AppDSL
    var paths: [String: [Route]] = [:]
    var selectedTab = ""
    var loading: Set<String> = []
    private let rows: [Record]

    init(dsl: AppDSL, rows: [Record]) {
        self.dsl = dsl
        self.rows = rows
    }

    func records(for entity: String) -> [Record] { rows }
    func load(entities: Set<String>, force: Bool) async {}
    func run(_ action: Action, _ ctx: ActionContext) async {}
    func close(_ route: Route) {}
    func pathBinding(_ stack: String) -> Binding<[Route]> { .constant([]) }
}
