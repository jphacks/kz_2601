import XCTest
import MetamonDSL
import MetamonRenderer
import MetamonServices
@testable import Metamon

/// #626: 生成中の「パーツを選んで置く」演出の段取り(置く順・時間の配分・Reduce Motion・編集で光らせる部品)。
@MainActor
final class BuildChoreographyTests: XCTestCase {
    private typealias C = BuildChoreography

    private func dsl(_ components: String, extraScreens: String = "") throws -> AppDSL {
        let json = """
        {"schemaVersion":1,"app":{"name":"t","voiceEntry":"none"},"initialScreen":"home","entities":[],
         "screens":[{"id":"home","title":"t","components":[\(components)]}\(extraScreens)]}
        """
        return try JSONDecoder().decode(AppDSL.self, from: Data(json.utf8))
    }

    // MARK: 依頼文からの当て推量

    func testHintsFollowWordOrderAndCapAtThree() {
        XCTAssertEqual(C.hints(for: "体重を記録してグラフで見たい"), [.field, .chart])
        XCTAssertEqual(C.hints(for: "やることをチェックする一覧と合計と写真"), [.toggle, .list, .stat])
        XCTAssertEqual(C.hints(for: "こんにちは"), [])
    }

    func testTrayPutsHintsFirstWithoutDuplicates() {
        let tray = C.tray(hints: [.chart, .field])
        XCTAssertEqual(tray.count, 6)
        XCTAssertEqual(Array(tray.prefix(2)), [.chart, .field])
        XCTAssertEqual(Set(tray).count, tray.count)
        XCTAssertEqual(C.tray(hints: []).first, .heading)
    }

    // MARK: 待っている間

    func testWaitingPlacesOneByOneThenSwapsReordersRelayouts() {
        let script = C.WaitingScript(transcript: "グラフで記録")
        let slots = C.WaitingScript.layouts[0].count
        for step in 0..<slots {
            let f = script.frame(at: step)
            XCTAssertEqual(f.placed.count, step + 1, "1 手で 1 つずつ置く")
            XCTAssertEqual(f.move, .place)
            XCTAssertEqual(f.picking, f.placed.last?.kind, "置いた部品は箱から選んだもの")
        }
        XCTAssertEqual(script.frame(at: 0).placed.first?.kind, .chart, "当て推量の部品から置く")
        XCTAssertEqual(script.frame(at: slots).move, .swap)
        XCTAssertEqual(script.frame(at: slots + 1).move, .reorder)
        XCTAssertEqual(script.frame(at: slots + 2).move, .relayout)
        XCTAssertEqual(script.frame(at: slots + 3).move, .swap)
    }

    func testWaitingFramesStayValid() {
        let script = C.WaitingScript(transcript: "家計簿")
        for step in 0..<80 {
            let f = script.frame(at: step)
            XCTAssertLessThanOrEqual(f.placed.count, C.WaitingScript.layouts[0].count)
            XCTAssertEqual(Set(f.placed.map(\.id)).count, f.placed.count, "id は重ならない(動きのアニメ用)")
            for (i, a) in f.placed.enumerated() {
                XCTAssertLessThanOrEqual(a.area.row2, GridArea.visibleRows)
                for b in f.placed[(i + 1)...] { XCTAssertFalse(a.area.overlaps(b.area), "step \(step): 部品が重なった") }
            }
        }
        // 並べ直しは id を保ったまま場所だけ変える
        let slots = C.WaitingScript.layouts[0].count
        let before = script.frame(at: slots), after = script.frame(at: slots + 1)
        XCTAssertEqual(Set(before.placed.map(\.id)), Set(after.placed.map(\.id)))
        XCTAssertNotEqual(before.placed.map(\.area), after.placed.map(\.area))
    }

    func testStepPacingSlowsWithReduceMotion() {
        XCTAssertEqual(C.WaitingScript.step(elapsed: 1.7, reduceMotion: false), 2)
        XCTAssertEqual(C.WaitingScript.step(elapsed: 1.7, reduceMotion: true), 0)
        XCTAssertEqual(C.WaitingScript.step(elapsed: -1, reduceMotion: false), 0)
    }

    // MARK: 結果が届いた時

    func testAssemblyOrdersTopToBottomThenLeftToRight() throws {
        let d = try dsl("""
        {"id":"add","type":"button","label":"記録する","area":"D11:F12"},
        {"id":"right","type":"stat","label":"合計","area":"D1:F2"},
        {"id":"left","type":"text","text":"家計簿","style":"title","area":"A1:C2"},
        {"id":"list","type":"list","area":"A3:F10"},
        {"id":"more","type":"chart"}
        """)
        let plan = C.AssemblyPlan.make(for: d, reduceMotion: false)
        XCTAssertEqual(plan.steps.map(\.part.id), ["left", "right", "list", "add", "more"])
        XCTAssertEqual(plan.steps.map(\.part.kind), [.heading, .stat, .list, .button, .chart])
        XCTAssertEqual(plan.steps.first?.part.caption, "家計簿")
        XCTAssertEqual(plan.steps[3].part.caption, "記録する")
        XCTAssertEqual(plan.steps[4].part.area, GridArea("A13:F16"), "area の無い部品は下に全幅で積む")
        XCTAssertEqual(plan.rows, 16)
        XCTAssertEqual(plan.tray, [.heading, .stat, .list, .button, .chart])
        let delays = plan.steps.map(\.delay)
        XCTAssertEqual(delays, delays.sorted(), "上から順に置く")
        XCTAssertLessThan(plan.steps.last!.delay, plan.total)
    }

    func testAssemblyTotalIsCappedForManyParts() throws {
        let many = (1...12).map { #"{"id":"t\#($0)","type":"text","text":"x","area":"A\#($0):F\#($0)"}"# }.joined(separator: ",")
        let plan = C.AssemblyPlan.make(for: try dsl(many), reduceMotion: false)
        XCTAssertLessThanOrEqual(plan.total, 1.5, "待たせすぎない")
        XCTAssertEqual(Set(plan.steps.map(\.delay)).count, C.AssemblyPlan.maxSteps, "上限を超えた分は最後の手でまとめて置く")
        let few = C.AssemblyPlan.make(for: try dsl(#"{"id":"a","type":"button","area":"A1:F2"}"#), reduceMotion: false)
        XCTAssertLessThan(few.total, plan.total, "部品が少なければ短い")
        XCTAssertGreaterThanOrEqual(few.total, C.AssemblyPlan.leadIn + C.AssemblyPlan.hold)
    }

    func testAssemblyWithReduceMotionFadesAllAtOnce() throws {
        let plan = C.AssemblyPlan.make(for: try DSLExamples.load("kakeibo"), reduceMotion: true)
        XCTAssertFalse(plan.steps.isEmpty)
        XCTAssertTrue(plan.steps.allSatisfy { $0.delay == 0 })
        XCTAssertEqual(plan.total, C.AssemblyPlan.reducedTotal)
    }

    func testAssemblyUsesInitialTabScreen() throws {
        let plan = C.AssemblyPlan.make(for: try DSLExamples.load("kakeibo"), reduceMotion: false)
        XCTAssertEqual(plan.steps.map(\.part.id), ["summary", "recent", "add"])
    }

    // MARK: 編集

    func testEditHighlightsOnlyChangedParts() throws {
        let base = """
        {"id":"a","type":"text","text":"見出し","area":"A1:F2"},
        {"id":"b","type":"stat","label":"合計","area":"A3:C5"},
        {"id":"c","type":"list","area":"A6:F12"}
        """
        let changed = """
        {"id":"a","type":"text","text":"見出し","area":"A1:F2"},
        {"id":"b","type":"stat","label":"今月の合計","area":"A3:C5"},
        {"id":"c","type":"list","area":"A6:F12"},
        {"id":"d","type":"chart","area":"D3:F5"}
        """
        let h = C.editHighlight(old: try dsl(base), new: try dsl(changed))
        XCTAssertEqual(h.components, ["home/b", "home/d"])
        XCTAssertEqual(h.screens, ["home"])
        XCTAssertTrue(C.editHighlight(old: try dsl(base), new: try dsl(base)).isEmpty, "変わっていなければ光らせない")
    }

    func testEditHighlightSkipsNewScreensAndRebuilds() throws {
        let old = try dsl(#"{"id":"a","type":"text","text":"x","area":"A1:F2"},{"id":"b","type":"list","area":"A3:F12"}"#)
        let rebuilt = try dsl(#"{"id":"x","type":"chart","area":"A1:F6"},{"id":"y","type":"button","area":"A7:F8"}"#,
                              extraScreens: #",{"id":"new","title":"n","components":[{"id":"z","type":"text","text":"n"}]}"#)
        XCTAssertTrue(C.editHighlight(old: old, new: rebuilt).isEmpty, "ほぼ全部が変わった画面・新しい画面は普段の出現に任せる")
    }

    func testEditHighlightIgnoresMissingIDs() throws {
        // id の無い部品は読むたびに UUID になる。中身が同じなら変わっていない扱い
        let comps = #"{"type":"text","text":"x","area":"A1:F2"},{"type":"list","area":"A3:F12"}"#
        XCTAssertTrue(C.editHighlight(old: try dsl(comps), new: try dsl(comps)).isEmpty)
    }

    // MARK: AppRuntime との繋ぎ

    func testCreateKeepsGeneratingViewForRevealThenFinishes() async throws {
        var services = BuildConfig.make(environment: .mock)
        services.ai = MockAIClient(delay: .milliseconds(50))
        let rt = AppRuntime(document: try DSLExamples.document(DSLExamples.initial, version: 0), services: services)
        rt.clarifyEnabled = false
        rt.reduceMotion = { true } // 演出を短く
        let initialID = rt.rootViewID
        await rt.submitUtterance("家計簿アプリを作って")
        XCTAssertNil(rt.morphJob)
        XCTAssertFalse(rt.dsl.isVoiceOnly, "結果は適用済み")
        let reveal = try XCTUnwrap(rt.buildReveal, "結果が届いたら組み上げの演出")
        XCTAssertEqual(rt.buildStage?.reveal, reveal)
        XCTAssertEqual(rt.rootViewID, initialID, "組み上げの間は生成中の表示を作り直さない")
        try await Task.sleep(for: .seconds(C.AssemblyPlan.reducedTotal + 0.4))
        XCTAssertNil(rt.buildReveal)
        XCTAssertNil(rt.buildStage)
        XCTAssertEqual(rt.rootViewID, "\(rt.document.appId)#\(rt.document.version)")
    }

    func testRevealCanBeSkippedAndIsDroppedByAnotherApply() async throws {
        var services = BuildConfig.make(environment: .mock)
        services.ai = MockAIClient(delay: .milliseconds(50))
        let rt = AppRuntime(document: try DSLExamples.document(DSLExamples.initial, version: 0), services: services)
        rt.clarifyEnabled = false
        await rt.submitUtterance("家計簿アプリを作って")
        XCTAssertNotNil(rt.buildReveal)
        rt.finishBuildReveal()
        XCTAssertNil(rt.buildReveal, "タップで飛ばせる")

        await rt.submitUtterance("筋トレの記録アプリを作って")
        XCTAssertNotNil(rt.buildReveal)
        rt.apply(try DSLExamples.document("walk", version: 9), animated: false)
        XCTAssertNil(rt.buildReveal, "別の版が当たったら組み上げは捨てる")
    }

    func testEditSetsHighlightNotReveal() async throws {
        var services = BuildConfig.make(environment: .mock)
        services.ai = MockAIClient(delay: .milliseconds(50))
        let rt = AppRuntime(document: try DSLExamples.document("kakeibo"), services: services)
        await rt.morph(transcript: "合計をもっと大きく")
        XCTAssertNil(rt.buildReveal, "編集は組み上げを挟まない")
        XCTAssertNil(rt.buildStage)
    }

    func testFailureLeavesNoStage() async throws {
        var services = BuildConfig.make(environment: .mock)
        services.ai = AlwaysFailingAI()
        let rt = AppRuntime(document: try DSLExamples.document(DSLExamples.initial, version: 0), services: services)
        rt.clarifyEnabled = false
        await rt.submitUtterance("家計簿アプリを作って")
        XCTAssertNil(rt.buildReveal)
        XCTAssertNil(rt.buildStage, "失敗したら生成中の表示は消えて元の画面(状態カード)へ")
        XCTAssertTrue(rt.dsl.isVoiceOnly)
    }
}

private struct AlwaysFailingAI: AIClient {
    func morph(transcript: String, appId: String, currentVersion: Int) async throws -> MorphResult {
        throw URLError(.badServerResponse)
    }
    func chat(appId: String, messages: [ChatMessage], locale: String?, mode: ChatMode?) async throws -> ChatTurn { throw URLError(.timedOut) }
    func resetChat(appId: String) async {}
}
