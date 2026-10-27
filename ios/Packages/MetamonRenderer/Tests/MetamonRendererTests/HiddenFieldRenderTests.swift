import XCTest
import SwiftUI
import Observation
import MetamonDSL
@testable import MetamonRenderer

/// テスト用のランタイム(レコードと自分の uid だけ持つ)。
@MainActor
@Observable
private final class QuizRuntime: DSLRuntime {
    var dsl: AppDSL
    var paths: [String: [Route]] = [:]
    var selectedTab = ""
    var loading: Set<String> = []
    var store: [String: [Record]]
    var currentUid: String?

    init(dsl: AppDSL, store: [String: [Record]], currentUid: String?) {
        self.dsl = dsl
        self.store = store
        self.currentUid = currentUid
    }

    func records(for entity: String) -> [Record] { store[entity] ?? [] }
    func load(entities: Set<String>, force: Bool) async {}
    func run(_ action: Action, _ ctx: ActionContext) async {}
    func close(_ route: Route) {}
    func pathBinding(_ stack: String) -> Binding<[Route]> { .constant([]) }
}

/// #262: hidden の field(クイズの正解)は作者のフォームにだけ出し、一覧・表など表示用レコードからは落とす。
final class HiddenFieldRenderTests: XCTestCase {
    private func quiz() throws -> AppDSL {
        var url = URL(fileURLWithPath: #filePath)
        for _ in 0..<6 { url.deleteLastPathComponent() }
        return try AppDSL.load(contentsOf: url.appending(path: "backend/internal/ai/examples/quiz.json"))
    }

    private let mine = Record(id: "q1", data: ["text": "日本でいちばん高い山は？", "correct": "A", "points": 10], author: "me")
    private let theirs = Record(id: "q2", data: ["text": "いちばん長い川は？", "correct": "B", "points": 10], author: "other")

    private func formFields(_ dsl: AppDSL) throws -> (correct: Component, text: Component) {
        let children = try XCTUnwrap(dsl.screen("question_form")?.components.first?.children)
        return (try XCTUnwrap(children.first { $0.field == "correct" }), try XCTUnwrap(children.first { $0.field == "text" }))
    }

    @MainActor func testFormHidesAnswerFromNonAuthor() throws {
        let dsl = try quiz()
        let entity = try XCTUnwrap(dsl.entity("questions"))
        let (correct, text) = try formFields(dsl)

        XCTAssertFalse(FormDraft(entity: entity).hides(correct, currentUid: "me"), "新規(問題を作る人)には正解欄を出す")
        XCTAssertFalse(FormDraft(entity: entity, record: mine).hides(correct, currentUid: "me"), "作者には出す")
        let others = FormDraft(entity: entity, record: theirs)
        XCTAssertTrue(others.hides(correct, currentUid: "me"), "作者以外には出さない")
        XCTAssertTrue(others.hides(correct, currentUid: nil), "自分が分からなければ出さない")
        XCTAssertFalse(others.hides(text, currentUid: "me"), "hidden でない項目は出す")
        XCTAssertEqual(others.values["correct"], "B","値はドラフトに残る(保存しても消えない)")
        XCTAssertEqual(others.payload["correct"], "B")

        let legacy = FormDraft(entity: entity, record: Record(id: "q3", data: ["correct": "C"]))
        XCTAssertFalse(legacy.hides(correct, currentUid: "me"), "author 無し(private / 未送信の自分の行)は出す")
    }

    @MainActor func testViewRecordsDropAnswerButGradingWorks() throws {
        let dsl = try quiz()
        let rt = QuizRuntime(dsl: dsl, store: [
            "questions": [mine, theirs],
            "players": [Record(id: "p1", data: ["name": "たろう"])],
            "answers": [Record(id: "a1", data: ["question": "q2", "player": "p1", "choice": "B"], author: "me")],
        ], currentUid: "me")

        XCTAssertTrue(rt.viewRecords(for: "questions").allSatisfy { $0.data["correct"] == nil }, "一覧・カレンダー等の表示用に正解を出さない")
        let list = try XCTUnwrap(dsl.screen("home")?.allComponents.first { $0.type == .list })
        XCTAssertEqual(rt.filteredRecords(list).map(\.id).sorted(), ["q1", "q2"])
        XCTAssertTrue(rt.filteredRecords(list).allSatisfy { $0.data["correct"] == nil })

        // 採点は元の値で動く(他人の問題に正解しても点が入る)
        let answer = try XCTUnwrap(rt.viewRecords(for: "answers").first)
        XCTAssertEqual(answer["is_correct"], .bool(true))
        XCTAssertEqual(answer["score"].doubleValue, 10)

        // filter は落とす前に当たる(hidden で絞っても値は出ない)
        let filtered = try JSONDecoder().decode(Component.self, from: Data(#"{"type":"list","entity":"questions","filter":{"field":"correct","op":"eq","value":"B"},"item":{"title":"text"}}"#.utf8))
        XCTAssertEqual(rt.filteredRecords(filtered).map(\.id), ["q2"])
        XCTAssertNil(rt.filteredRecords(filtered).first?.data["correct"])

        // 回答の「問題」(ref)の表示名は問題文
        XCTAssertEqual(rt.refTitle("q2", entity: "questions"), "いちばん長い川は？")
    }
}
