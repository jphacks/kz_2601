import XCTest
@testable import MetamonDSL

/// #262: 回答者に見せない項目 `Field.hidden`(クイズの正解)。
final class HiddenFieldTests: XCTestCase {
    /// backend の例(`backend/internal/ai/examples/quiz.json`)をそのまま読む。
    private func quiz() throws -> AppDSL {
        try AppDSL.load(contentsOf: ExamplesFixture.repoRoot.appending(path: "backend/internal/ai/examples/quiz.json"))
    }

    func testDecodesHidden() throws {
        let questions = try XCTUnwrap(try quiz().entity("questions"))
        XCTAssertEqual(questions.field("correct")?.hidden, true)
        XCTAssertEqual(questions.field("text")?.hidden, false, "省略時は false")
        XCTAssertEqual(questions.hiddenFieldIds, ["correct"])
    }

    func testDisplayTitleFieldSkipsHidden() throws {
        let json = #"{"id":"q","name":"問題","titleField":"secret","fields":[{"id":"secret","type":"string","hidden":true},{"id":"text","type":"string"}]}"#
        let entity = try JSONDecoder().decode(Entity.self, from: Data(json.utf8))
        XCTAssertEqual(entity.displayTitleField, "text", "hidden を見出し(ref の表示名)にしない")
    }

    func testCanEditOnlyForAuthor() {
        XCTAssertTrue(HiddenFields.canEdit(record: nil, currentUid: "me"), "新規は入力する本人が作者")
        XCTAssertTrue(HiddenFields.canEdit(record: Record(id: "1", data: [:]), currentUid: "me"), "author 無し = private / 未送信の自分の行")
        XCTAssertTrue(HiddenFields.canEdit(record: Record(id: "1", data: [:], author: "me"), currentUid: "me"))
        XCTAssertFalse(HiddenFields.canEdit(record: Record(id: "1", data: [:], author: "other"), currentUid: "me"))
        XCTAssertFalse(HiddenFields.canEdit(record: Record(id: "1", data: [:], author: "other"), currentUid: nil), "自分が分からなければ出さない")
    }

    func testRecordDecodesAuthor() throws {
        let r = try JSONDecoder().decode(Record.self, from: Data(#"{"id":"1","data":{"text":"q"},"author":"u1","authorName":"a"}"#.utf8))
        XCTAssertEqual(r.author, "u1")
    }

    /// 表示用は正解を落とすが、採点(eq で比べる computed)は元の値で動く。
    func testGradingReadsHiddenButViewRedacts() throws {
        let dsl = try quiz()
        let records: [String: [Record]] = [
            "questions": [Record(id: "q1", data: ["text": "富士山の高さは？", "correct": "B", "points": 10], author: "teacher")],
            "players": [Record(id: "p1", data: ["name": "たろう"])],
            "answers": [
                Record(id: "a1", data: ["question": "q1", "player": "p1", "choice": "B"]),
                Record(id: "a2", data: ["question": "q1", "player": "p1", "choice": "A"]),
            ],
        ]
        let ev = ExprEvaluator(dsl: dsl, records: { records[$0] ?? [] })
        let answers = ev.viewRecords(for: "answers")
        XCTAssertEqual(answers.first { $0.id == "a1" }?["is_correct"], .bool(true))
        XCTAssertEqual(answers.first { $0.id == "a1" }?["score"].doubleValue, 10)
        XCTAssertEqual(answers.first { $0.id == "a2" }?["is_correct"], .bool(false))
        XCTAssertEqual(answers.first { $0.id == "a2" }?["score"].doubleValue, 0)
        XCTAssertEqual(ev.viewRecords(for: "players").first?["total"].doubleValue, 10, "合計点も正解を読んで出る")

        let shown = HiddenFields.redact(ev.viewRecords(for: "questions"), in: dsl.entity("questions"))
        XCTAssertNil(shown.first?.data["correct"], "表示用から正解を落とす")
        XCTAssertEqual(shown.first?["text"], "富士山の高さは？")
        XCTAssertEqual(shown.first?.author, "teacher")
        XCTAssertEqual(HiddenFields.redact(["correct": "B", "text": "q"], in: dsl.entity("questions")), ["text": "q"])
    }
}
