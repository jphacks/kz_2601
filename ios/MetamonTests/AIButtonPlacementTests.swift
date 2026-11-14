import XCTest
import MetamonDSL
import MetamonRenderer
import MetamonServices
@testable import Metamon

/// どの画面・シートでも AI ボタンを出す(#629): 層ごとの入口・今見えている画面・AI に渡す文脈。
@MainActor
final class AIButtonPlacementTests: XCTestCase {
    override func tearDown() {
        ScreenMapUI.shared.context = nil
        super.tearDown()
    }

    // MARK: - 入口(層ごと)

    func testMainLayerShowsFabOnNonFormScreen() {
        XCTAssertEqual(entry(.main), .fab)
        // フォームはナビバーのマイク(保存・削除に FAB が被るため・#83)
        XCTAssertEqual(entry(.main, isForm: true), .toolbar)
    }

    func testModalLayerShowsSameFab() {
        XCTAssertEqual(entry(.modal), .fab)
        XCTAssertEqual(entry(.modal, isForm: true), .toolbar)
    }

    func testComponentSheetUsesToolbarMic() {
        XCTAssertEqual(entry(.component), .toolbar)
        // キーボード表示中もナビバーのマイクは出したまま(キーボードに被らない)
        XCTAssertEqual(entry(.component, keyboardVisible: true), .toolbar)
    }

    func testFabHidesWhileKeyboardVisible() {
        XCTAssertEqual(entry(.main, keyboardVisible: true), Entry.none)
        XCTAssertEqual(entry(.modal, keyboardVisible: true), Entry.none)
        XCTAssertEqual(entry(.modal, isForm: true, keyboardVisible: true), .toolbar)
    }

    func testCoveredLayerShowsNothing() {
        // 上にシート・部品のシートがある層は出さない(下に隠れた FAB を VoiceOver が拾わないように)
        XCTAssertEqual(entry(.main, isTopLayer: false), Entry.none)
        XCTAssertEqual(entry(.modal, isTopLayer: false), Entry.none)
        XCTAssertEqual(entry(.modal, isForm: true, isTopLayer: false), Entry.none)
    }

    func testVoiceEntryRules() {
        // 初期画面(音声だけ)はどの層でも出さない
        XCTAssertEqual(entry(.main, voiceEntry: .fullscreen), Entry.none)
        XCTAssertEqual(entry(.component, voiceEntry: .fullscreen), Entry.none)
        // FAB を出さない設定(none)でも、フォームのマイクは従来どおり出す
        XCTAssertEqual(entry(.main, voiceEntry: VoiceEntry.none), Entry.none)
        XCTAssertEqual(entry(.main, voiceEntry: VoiceEntry.none, isForm: true), .toolbar)
    }

    func testNothingWhileBuilding() {
        // 組み上げ中(#626 `rt.buildStage != nil`)は根の FAB・シートの FAB・部品のシートのマイク・フォームのマイクのどれも出さない
        XCTAssertEqual(entry(.main, isBuilding: true), Entry.none)
        XCTAssertEqual(entry(.modal, isBuilding: true), Entry.none)
        XCTAssertEqual(entry(.component, isBuilding: true), Entry.none)
        XCTAssertEqual(entry(.main, isForm: true, isBuilding: true), Entry.none)
    }

    // MARK: - 今見えている画面

    func testVisiblePrefersModalOverMainStack() {
        let main = [Route(screen: "detail", stack: "main")]
        let sheet = Route(screen: "pick", stack: "sheet")
        var v = visible(paths: ["main": main])
        XCTAssertEqual(v, .init(route: main[0], layer: .main, isStackRoot: false))

        v = visible(paths: ["main": main], sheet: sheet)
        XCTAssertEqual(v, .init(route: sheet, layer: .modal, isStackRoot: true))

        let pushedInSheet = Route(screen: "pickDetail", stack: "sheet")
        v = visible(paths: ["main": main, "sheet": [pushedInSheet]], sheet: sheet)
        XCTAssertEqual(v, .init(route: pushedInSheet, layer: .modal, isStackRoot: false))

        let full = Route(screen: "camera", stack: "full")
        XCTAssertEqual(visible(paths: [:], fullScreen: full).route, full)
    }

    func testVisibleRootAndTabs() {
        XCTAssertEqual(visible(paths: [:]), .init(route: Route(screen: "home"), layer: .main, isStackRoot: true))
        let tab = Route.tabStack("stats")
        let v = AIButtonPlacement.visible(paths: [tab: []], selectedTab: "stats", hasTabs: true, initialScreen: "home",
                                          sheet: nil, fullScreen: nil)
        XCTAssertEqual(v, .init(route: Route(screen: "stats", stack: tab), layer: .main, isStackRoot: true))
    }

    // MARK: - AI に渡す文脈

    func testContextForPushedModalAndComponent() {
        let pushed = AIButtonPlacement.Visible(route: Route(screen: "detail"), layer: .main, isStackRoot: false)
        XCTAssertEqual(AIButtonPlacement.context(visible: pushed, hasTabs: false, screenTitle: "詳細", component: nil),
                       ScreenContext(id: "detail", title: "詳細"))

        let modal = AIButtonPlacement.Visible(route: Route(screen: "pick", stack: "sheet"), layer: .modal, isStackRoot: true)
        // タイトルが空なら id を見せる
        XCTAssertEqual(AIButtonPlacement.context(visible: modal, hasTabs: false, screenTitle: "", component: nil)?.title, "pick")

        let part = AIButtonComponent(id: "where", label: "場所")
        XCTAssertEqual(AIButtonPlacement.context(visible: modal, hasTabs: false, screenTitle: "選ぶ", component: part),
                       ScreenContext(id: "pick", title: "選ぶ", component: part))
    }

    func testNoContextOnHomeOfTablessApp() {
        let home = AIButtonPlacement.Visible(route: Route(screen: "home"), layer: .main, isStackRoot: true)
        // アプリ全体への指示を 1 画面に縛らない(従来どおり)
        XCTAssertNil(AIButtonPlacement.context(visible: home, hasTabs: false, screenTitle: "ホーム", component: nil))
        // タブがあれば「ここに」がどのタブか分かるように付ける
        XCTAssertEqual(AIButtonPlacement.context(visible: home, hasTabs: true, screenTitle: "ホーム", component: nil)?.id, "home")
        // 部品のシートからなら根でも付ける
        let part = AIButtonComponent(id: "music", label: "音楽")
        XCTAssertEqual(AIButtonPlacement.context(visible: home, hasTabs: false, screenTitle: "ホーム", component: part)?.component, part)
    }

    func testContextPrefixNamesScreenAndComponent() {
        let screen = ScreenContext(id: "detail", title: "詳細")
        let (title, id, label, cid) = ("詳細", "detail", "場所", "where")
        XCTAssertEqual(ScreenMapCopy.contextPrefix(screen), L10n.tr("画面「\(title)」(id: \(id))で: "))
        let part = ScreenContext(id: "detail", title: "詳細", component: AIButtonComponent(id: "where", label: "場所"))
        let prefix = ScreenMapCopy.contextPrefix(part)
        XCTAssertEqual(prefix, L10n.tr("画面「\(title)」(id: \(id))の部品「\(label)」(id: \(cid))で: "))
        XCTAssertNotEqual(prefix, ScreenMapCopy.contextPrefix(screen))
        XCTAssertNotEqual(ScreenMapCopy.chipText(part), ScreenMapCopy.chipText(screen))
    }

    /// Mac の部品の「この部品について話す」(#1323)も、iOS の部品のシートと同じ文脈・同じ文言の先頭になる
    func testComponentTalkTargetBuildsSameContext() throws {
        let dsl = try runtime().dsl
        let part = AIButtonComponent(id: "where", label: "場所")
        let screen = ScreenContext(ComponentTalk.Target(route: Route(screen: "detail", stack: "sheet"), component: part), dsl: dsl)
        XCTAssertEqual(screen, ScreenContext(id: "detail", title: "詳細", component: part))
        let (title, id, label, cid) = ("詳細", "detail", "場所", "where")
        XCTAssertEqual(ScreenMapCopy.contextPrefix(screen), L10n.tr("画面「\(title)」(id: \(id))の部品「\(label)」(id: \(cid))で: "))
        XCTAssertFalse(ScreenMapCopy.talkAboutComponent.isEmpty)
    }

    func testComponentLabelFallsBack() throws {
        let json = #"[{"id":"a","type":"locationPicker","label":"行き先"},{"id":"b","type":"musicPlayer","title":"BGM"},{"id":"c","type":"dashcam"}]"#
        let comps = try JSONDecoder().decode([Component].self, from: Data(json.utf8))
        XCTAssertEqual(comps.map { AIButtonComponent($0).label }, ["行き先", "BGM", "dashcam"])
    }

    // MARK: - 押した時(AppRuntime)

    func testOpenVoiceSetsContextOfVisibleScreen() throws {
        let rt = try runtime()
        rt.openVoiceForVisibleScreen()
        XCTAssertTrue(rt.showVoiceSheet)
        XCTAssertNil(ScreenMapUI.shared.context, "タブの無いアプリのホームは文脈なし")

        rt.showVoiceSheet = false
        rt.paths["main"] = [Route(screen: "detail")]
        rt.openVoiceForVisibleScreen()
        XCTAssertEqual(ScreenMapUI.shared.context, ScreenContext(id: "detail", title: "詳細"))

        rt.showVoiceSheet = false
        rt.sheet = Route(screen: "detail", stack: "sheet")
        let part = AIButtonComponent(id: "where", label: "場所")
        rt.openVoiceForVisibleScreen(component: part)
        XCTAssertEqual(ScreenMapUI.shared.context, ScreenContext(id: "detail", title: "詳細", component: part))
    }

    func testLayersTopIsLastPushed() {
        let layers = AIButtonLayers()
        let a = UUID(), b = UUID()
        XCTAssertTrue(layers.isEmpty)
        layers.push(a); layers.push(b); layers.push(b)
        XCTAssertTrue(layers.isTop(b))
        XCTAssertFalse(layers.isTop(a))
        layers.remove(b)
        XCTAssertTrue(layers.isTop(a))
        layers.remove(a)
        XCTAssertTrue(layers.isEmpty)
    }

    // MARK: - helpers

    private typealias Entry = AIButtonPlacement.Entry

    private func entry(_ layer: AIButtonPlacement.Layer, voiceEntry: VoiceEntry = .floating, isForm: Bool = false,
                       keyboardVisible: Bool = false, isTopLayer: Bool = true, isBuilding: Bool = false) -> Entry {
        AIButtonPlacement.entry(layer: layer, voiceEntry: voiceEntry, isForm: isForm, keyboardVisible: keyboardVisible,
                                isTopLayer: isTopLayer, isBuilding: isBuilding)
    }

    private func visible(paths: [String: [Route]], sheet: Route? = nil, fullScreen: Route? = nil) -> AIButtonPlacement.Visible {
        AIButtonPlacement.visible(paths: paths, selectedTab: "", hasTabs: false, initialScreen: "home", sheet: sheet, fullScreen: fullScreen)
    }

    private func runtime() throws -> AppRuntime {
        let json = """
        {"schemaVersion":1,"app":{"name":"メモ","voiceEntry":"floating"},"initialScreen":"home",
         "entities":[{"id":"notes","name":"メモ","fields":[{"id":"body","name":"本文","type":"text"}]}],
         "screens":[{"id":"home","title":"ホーム","components":[{"type":"list","entity":"notes"}]},
                    {"id":"detail","title":"詳細","components":[{"type":"text","text":"x"}]}]}
        """
        let dsl = try JSONDecoder().decode(AppDSL.self, from: Data(json.utf8))
        let appId = "test-aibutton-\(UUID().uuidString.lowercased())"
        return AppRuntime(document: AppDocument(appId: appId, version: 1, dsl: dsl), services: BuildConfig.make(environment: .mock))
    }
}
