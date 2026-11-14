import XCTest
import MetamonDSL
import MetamonRenderer
import MetamonServices
@testable import Metamon

/// #636: SSE で届いた本物の部品を組み立て台に仮置きし、restart で捨て、最後の結果で置き直す。
@MainActor
final class BuildLiveDraftTests: XCTestCase {
    private typealias C = BuildChoreography

    private func comp(_ index: Int, _ type: String, area: String? = nil, id: String? = nil, label: String? = nil, attempt: Int = 1) -> GenerateProgress {
        .component(StreamedComponent(attempt: attempt, screen: "home", index: index, id: id, type: type, area: area, label: label))
    }

    func testComponentsReplaceGuessesInArrivalOrderOncePerIndex() {
        var d = C.LiveDraft()
        d.apply(.app(name: "家計簿", tabs: ["ホーム", "集計"], attempt: 1))
        d.apply(comp(0, "text", area: "A1:F1", id: "title", label: "家計簿"))
        d.apply(comp(1, "stat", area: "A2:C4", id: "total", label: "今月の合計はいくらでしょう"))
        d.apply(comp(1, "chart", area: "A2:C4", id: "other")) // 同じ index は 1 回だけ
        XCTAssertEqual(d.appName, "家計簿")
        XCTAssertEqual(d.tabs, ["ホーム", "集計"])
        XCTAssertEqual(d.parts.map(\.id), ["title", "total"])
        XCTAssertEqual(d.parts.map(\.kind), [.heading, .stat], "1 行の text は見出し")
        XCTAssertEqual(d.parts[1].caption, "今月の合計はいくらでし…", "長い名前は詰める")
        XCTAssertEqual(d.latestKind, .stat)
        XCTAssertEqual(d.tray(base: [.list, .stat]).prefix(3), [.heading, .stat, .list])

        let script = C.WaitingScript(transcript: "グラフで記録")
        let frame = script.frame(at: 3, live: d)
        XCTAssertEqual(Array(frame.placed.prefix(2).map(\.id)), ["title", "total"], "本物を先に置く")
        for g in frame.placed.dropFirst(2) {
            XCTAssertFalse(d.parts.contains { $0.area.overlaps(g.area) }, "下書きは本物と重ならない所だけ")
        }
        XCTAssertLessThanOrEqual(frame.placed.count, C.WaitingScript.layouts[0].count)
        XCTAssertEqual(frame.picking, .stat)
        XCTAssertEqual(script.frame(at: 3, live: C.LiveDraft()), script.frame(at: 3), "届く前は今までどおり")
    }

    func testPartsWithoutAreaStackBelowAndUnknownTypesAreCards() {
        var d = C.LiveDraft()
        d.apply(comp(0, "list"))
        d.apply(comp(1, "futureThing", area: "A1:F2"))
        let parts = d.parts
        XCTAssertEqual(parts.map(\.kind), [.card, .list])
        XCTAssertEqual(parts[1].area, GridArea("A3:F6"), "area の無い部品は下に全幅で積む")
        XCTAssertEqual(parts[1].id, "stream-0")
    }

    func testRestartDropsProvisionalPartsAndOldAttemptsAreIgnored() {
        var d = C.LiveDraft()
        d.apply(.app(name: "家計簿", tabs: [], attempt: 1))
        d.apply(comp(0, "list", area: "A1:F6"))
        d.apply(.restart(attempt: 2))
        XCTAssertTrue(d.parts.isEmpty, "作り直しで仮置きを捨てる")
        let script = C.WaitingScript(transcript: "家計簿")
        XCTAssertEqual(script.frame(at: 2, live: d), script.frame(at: 2), "当て推量に戻る")
        d.apply(comp(1, "chart", area: "A1:F4", attempt: 1)) // 前の回の遅れて届いた部品
        XCTAssertTrue(d.parts.isEmpty)
        d.apply(comp(0, "chart", area: "A1:F4", attempt: 2))
        XCTAssertEqual(d.parts.map(\.kind), [.chart])
        d.apply(comp(0, "stat", area: "A1:F2", attempt: 3)) // restart を取りこぼしても新しい回で前の回を捨てる
        XCTAssertEqual(d.parts.map(\.kind), [.stat])
        d.apply(.restart(attempt: 0)) // iOS 側のやり直し: 次の stream は 1 回目から
        d.apply(comp(0, "list", attempt: 1))
        XCTAssertEqual(d.parts.map(\.kind), [.list])
    }

    func testResultKeepsMatchingProvisionalPartsAndPlacesTheRest() throws {
        let json = """
        {"schemaVersion":1,"app":{"name":"t","voiceEntry":"none","tabs":[{"title":"ホーム","screen":"home"},{"title":"集計","screen":"stats"}]},
         "initialScreen":"home","entities":[],
         "screens":[{"id":"home","title":"t","components":[
           {"id":"title","type":"text","text":"家計簿","area":"A1:F1"},
           {"id":"list","type":"list","area":"A2:F8"},
           {"id":"add","type":"button","label":"追加","area":"A9:F10"}]},
          {"id":"stats","title":"s","components":[]}]}
        """
        let dsl = try JSONDecoder().decode(AppDSL.self, from: Data(json.utf8))
        var d = C.LiveDraft()
        d.apply(comp(0, "text", area: "A1:F1", id: "title"))
        d.apply(comp(1, "chart", area: "A2:F8", id: "list")) // 結果と違う(結果で置き直す)
        let plan = C.AssemblyPlan.make(for: dsl, reduceMotion: false, shown: d.parts)
        XCTAssertEqual(plan.steps.map(\.part.id), ["title", "list", "add"])
        XCTAssertEqual(plan.steps[0].delay, 0, "仮置きと同じ部品は置いたまま")
        XCTAssertGreaterThan(plan.steps[1].delay, 0, "違った部品は結果で置き直す")
        XCTAssertEqual(plan.steps[1].part.kind, .list)
        XCTAssertEqual(plan.tabs, ["ホーム", "集計"])
        XCTAssertEqual(C.AssemblyPlan.make(for: dsl, reduceMotion: false), C.AssemblyPlan.make(for: dsl, reduceMotion: false, shown: []))
    }

    // MARK: AppRuntime との繋ぎ

    func testRuntimeShowsProvisionalPartsWhileWaitingThenRevealsResult() async throws {
        var services = BuildConfig.make(environment: .mock)
        let ai = StreamingAI()
        services.ai = ai
        let rt = AppRuntime(document: try DSLExamples.document(DSLExamples.initial, version: 0), services: services)
        rt.clarifyEnabled = false
        rt.reduceMotion = { true }
        let run = Task { await rt.submitUtterance("家計簿アプリを作って") }
        var draft: C.LiveDraft?
        for _ in 0..<200 {
            if let d = rt.morphJob?.draft, d.parts.count == 1 { draft = d; break }
            try await Task.sleep(for: .milliseconds(10))
        }
        let shown = try XCTUnwrap(draft, "待っている間に仮置きが出る")
        XCTAssertEqual(shown.appName, "家計簿")
        XCTAssertEqual(rt.buildStage?.job.draft.parts.first?.kind, .list)
        ai.release()
        await run.value
        XCTAssertNil(rt.morphJob)
        XCTAssertNotNil(rt.buildReveal, "結果で組み上げに渡す")
        XCTAssertFalse(rt.dsl.isVoiceOnly)
    }

    func testEditDoesNotAskForProgress() async throws {
        var services = BuildConfig.make(environment: .mock)
        let ai = StreamingAI()
        ai.release()
        services.ai = ai
        let rt = AppRuntime(document: try DSLExamples.document("kakeibo"), services: services)
        await rt.morph(transcript: "合計をもっと大きく")
        XCTAssertEqual(ai.progressCalls, 0, "edit は今のまま(stream しない)")
    }
}

/// 途中経過を流してから、release されるまで結果を返さない偽の AI。
private final class StreamingAI: AIClient, @unchecked Sendable {
    private let lock = NSLock()
    private var released = false
    private(set) var progressCalls = 0

    func release() { lock.withLock { released = true } }
    private var isReleased: Bool { lock.withLock { released } }

    func morph(transcript: String, appId: String, currentVersion: Int) async throws -> MorphResult {
        try await MockAIClient(delay: .milliseconds(10)).morph(transcript: transcript, appId: appId, currentVersion: currentVersion)
    }

    func morph(transcript: String, appId: String, currentVersion: Int, locale: String?,
               progress: @escaping @Sendable (GenerateProgress) -> Void) async throws -> MorphResult {
        lock.withLock { progressCalls += 1 }
        progress(.app(name: "家計簿", tabs: [], attempt: 1))
        progress(.component(StreamedComponent(attempt: 1, screen: "home", index: 0, type: "list", area: "A1:F6")))
        while !isReleased { try await Task.sleep(for: .milliseconds(10)) }
        return try await morph(transcript: transcript, appId: appId, currentVersion: currentVersion)
    }

    func chat(appId: String, messages: [ChatMessage], locale: String?, mode: ChatMode?) async throws -> ChatTurn { throw URLError(.timedOut) }
    func resetChat(appId: String) async {}
}
