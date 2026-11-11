import XCTest
import SwiftUI
import Observation
import MetamonDSL
@testable import MetamonRenderer
#if os(macOS)
import AppKit
#endif

/// #941: 表示だけの Leaf 15 種(text・label・markdown・image・icon・avatar・badge・divider・spacer・
/// emptyState・link・timeline・rating・progressBar・gauge)が Mac でも iOS と同じ内容で描け、VoiceOver が読む名前を持つ。
/// 判定は両方の OS で回し、ウインドウに置く確認と記号の差し替えは Mac だけで確かめる。
@MainActor
final class MacDisplayLeavesTests: XCTestCase {
    /// この Issue で見る 15 種。
    static let displayTypes = ["text", "label", "markdown", "image", "icon", "avatar", "badge", "divider",
                               "spacer", "emptyState", "link", "timeline", "rating", "progressBar", "gauge"]

    // MARK: - 15 種が描ける

    func testAllDisplayLeavesAreRegistered() {
        for type in Self.displayTypes {
            XCTAssertNotNil(ComponentRegistry.standard.builder(for: type), type)
        }
    }

    /// 15 種すべてが落ちずに絵になる(iOS は UIImage・Mac は NSImage)。
    func testEveryDisplayLeafRenders() throws {
        let fixture = try Fixture()
        for c in fixture.components {
            let view = try XCTUnwrap(ComponentRegistry.standard.builder(for: c.rawType), c.rawType)(c, fixture.context)
            let image = ImageRenderer(content: view.environment(\.dslRuntime, fixture.runtime).frame(width: 360, height: 160)).platformImage
            XCTAssertNotNil(image, c.rawType)
        }
        XCTAssertEqual(fixture.components.map(\.rawType), Self.displayTypes, "15 種をそろえて確かめる")
    }

    // MARK: - Mac に無い SF Symbols の代わり

    func testUnknownSymbolFallsBackToOneThatExists() {
        XCTAssertEqual(FieldIcon.resolve("heart.fill", fallback: "circle"), "heart.fill")
        XCTAssertEqual(FieldIcon.resolve("zzz.not.a.real.symbol", fallback: "circle"), "circle", "この OS に無い名前は代わりの記号")
        XCTAssertEqual(FieldIcon.resolve(nil, fallback: "circle"), "circle")
        XCTAssertEqual(FieldIcon.resolve("", fallback: "circle"), "circle")
        XCTAssertTrue(FieldIcon.exists("star"))
        XCTAssertFalse(FieldIcon.exists("zzz.not.a.real.symbol"))
        XCTAssertFalse(FieldIcon.exists("zzz.not.a.real.symbol"), "2 回目は覚えた答えを返す")
    }

    /// DSL の `icon` が Mac に無い名前でも、field から推定した記号に落ちる(空白の四角にしない)。
    func testComponentIconFallsBackToInferredSymbol() throws {
        let field = try JSONDecoder().decode(Field.self, from: Data(#"{"id":"when","name":"日付","type":"date"}"#.utf8))
        let bad = try JSONDecoder().decode(Component.self, from: Data(#"{"type":"text","icon":"zzz.not.a.real.symbol","field":"when"}"#.utf8))
        XCTAssertEqual(FieldIcon.infer(component: bad, field: field), "calendar")
        let good = try JSONDecoder().decode(Component.self, from: Data(#"{"type":"text","icon":"flame","field":"when"}"#.utf8))
        XCTAssertEqual(FieldIcon.infer(component: good, field: field), "flame")
    }

    /// FieldIcon が返しうる記号は、動かしている OS に実在する(Mac で欠けていたら気づく)。
    func testInferredSymbolsExistOnThisPlatform() throws {
        let types: [String] = ["currency", "date", "datetime", "enum", "photo", "video", "audio", "location", "boolean", "text", "integer", "number", "string"]
        var names: Set<String> = []
        for type in types {
            let field = try JSONDecoder().decode(Field.self, from: Data(#"{"id":"f","name":"n","type":"\#(type)"}"#.utf8))
            for hint in ["", "時刻", "weight", "回", "歩", "メモ", "password", "mail", "url", "電話", "名前", "検索"] {
                names.insert(FieldIcon.infer(field: field, hint: hint))
            }
        }
        for aggregate in ["count", "avg", "max", "min", "distance", "duration", "sum"] {
            names.insert(FieldIcon.stat(aggregate: aggregate, field: nil))
        }
        for name in names.sorted() {
            XCTAssertTrue(FieldIcon.exists(name), "この OS に無い記号: \(name)")
        }
    }

    // MARK: - markdown のリンク

    /// 許可されたスキームのリンクだけ生かす。生きたリンクは `openURL`(Mac は既定のブラウザ)が開く。
    func testMarkdownKeepsOnlyOpenableLinks() {
        let out = MarkdownLeaf.attributed("**強調** [公式](https://example.com/a) [罠](javascript:alert(1)) [相対](docs/a.md) [電話](tel:0120000000)")
        let links = out.runs.compactMap { run in run.link.map { (String(out[run.range].characters), $0.absoluteString) } }
        XCTAssertEqual(links.map(\.0), ["公式", "電話"], "https と allowlist のスキームだけリンクにする")
        XCTAssertEqual(links.first?.1, "https://example.com/a")
        XCTAssertTrue(String(out.characters).contains("罠"), "外したリンクも文字は残す")
        XCTAssertTrue(String(out.characters).contains("相対"))
        XCTAssertFalse(String(out.characters).contains("**"), "太字は解釈する")
        XCTAssertEqual(String(MarkdownLeaf.attributed("素の文です").characters), "素の文です")
    }

    // MARK: - rating をマウス・キーボードで

    func testRatingClampsToRange() {
        XCTAssertEqual(RatingLeaf.clamp(3, count: 5), 3)
        XCTAssertEqual(RatingLeaf.clamp(6, count: 5), 5, "→ キーが端を越えない")
        XCTAssertEqual(RatingLeaf.clamp(-1, count: 5), 0, "← キーで 0(未評価)まで")
        XCTAssertEqual(RatingLeaf.clamp(0, count: 5), 0)
    }

    // MARK: - VoiceOver が読む文言

    /// VoiceOver に渡す文言が日本語・英語の両方で出る(キーのまま読まれない)。
    /// avatar は名前が無ければ「アバター」、rating は「評価」、link は行き先の説明。
    func testVoiceOverCopyIsLocalized() {
        let saved = LanguagePreference.shared.setting
        defer { LanguagePreference.shared.setting = saved }
        LanguagePreference.shared.setting = .ja
        XCTAssertEqual(L("アバター"), "アバター")
        XCTAssertEqual(L("評価"), "評価")
        XCTAssertEqual(L("ブラウザで開きます"), "ブラウザで開きます")
        LanguagePreference.shared.setting = .en
        XCTAssertEqual(L("アバター"), "Avatar")
        XCTAssertEqual(L("評価"), "Rating")
        XCTAssertEqual(L("ブラウザで開きます"), "Opens in your browser")
    }

    /// progressBar・gauge が VoiceOver に渡す値の文言(「—」と「42%」を取り違えない)。
    func testProgressAndGaugeSpeakTheirValue() {
        XCTAssertEqual(ProgressBarLeaf.percentText(raw: 0.42, tone: nil), "42%")
        XCTAssertEqual(ProgressBarLeaf.percentText(raw: 1.12, tone: "negative"), "112%")
        XCTAssertEqual(EmptyPlaceholder.unavailableValue, "—")
        XCTAssertEqual(Aggregator.format(62.5, as: nil, unit: nil), "62.5")
    }

    #if os(macOS)

    // MARK: - Mac のウインドウで

    /// 15 種が Mac のウインドウに置けて、高さを持つ(描けている)。
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
