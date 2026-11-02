import XCTest
@testable import MetamonDSL

/// `flashcards`(#264): 例のデコード・覚えた / まだ の filter・並び・めくる・答える(保存の差分)・進み具合・完了・もう一度。
final class FlashcardDeckTests: XCTestCase {
    private let words = [
        Record(id: "w3", data: ["term": "consider"], createdAt: "2026-11-03T00:00:00Z"),
        Record(id: "w1", data: ["term": "apple", "learned": .bool(false)], createdAt: "2026-11-01T00:00:00Z"),
        Record(id: "w2", data: ["term": "borrow", "learned": .bool(true)], createdAt: "2026-11-02T00:00:00Z"),
    ]

    func testExampleDecodes() throws {
        let dsl = try ExamplesFixture.load("vocab")
        let c = try XCTUnwrap(dsl.screens.flatMap(\.allComponents).first { $0.type == .flashcards })
        XCTAssertEqual(c.entity, "word")
        XCTAssertEqual(c.front, "term")
        XCTAssertEqual(c.back, "meaning")
        XCTAssertEqual(c.toggle, "learned")
        XCTAssertEqual(c.filter, Filter(field: "learned", op: "eq", value: .bool(false)))
        XCTAssertTrue(DSLExamples.names.contains("vocab"))
    }

    func testLearnedFilter() {
        XCTAssertEqual(FlashcardDeck.learnedFilter(Filter(field: "learned", op: "eq", value: .bool(false)), toggle: "learned"), false)
        XCTAssertEqual(FlashcardDeck.learnedFilter(Filter(field: "learned", op: "neq", value: .bool(true)), toggle: "learned"), false)
        XCTAssertEqual(FlashcardDeck.learnedFilter(Filter(field: "learned", op: "eq", value: .bool(true)), toggle: "learned"), true)
        XCTAssertEqual(FlashcardDeck.learnedFilter(Filter(field: "learned", op: "eq", value: .string("false")), toggle: "learned"), false)
        XCTAssertNil(FlashcardDeck.learnedFilter(Filter(field: "level", op: "eq", value: .bool(false)), toggle: "learned"), "toggle 以外の field")
        XCTAssertNil(FlashcardDeck.learnedFilter(Filter(field: "learned", op: "eq", value: .bool(false)), toggle: nil), "toggle 無し")
        XCTAssertNil(FlashcardDeck.learnedFilter(Filter(field: "learned", op: "gt", value: .bool(false)), toggle: "learned"))
    }

    /// まだのカードだけ(値の無いカードも「まだ」)・登録順。
    func testFilterExcludesLearnedAndKeepsOrder() {
        let unlearned = FlashcardDeck.cards(words, toggle: "learned", learned: false)
        XCTAssertEqual(unlearned.map(\.id), ["w1", "w3"])
        XCTAssertEqual(FlashcardDeck.cards(words, toggle: "learned", learned: true).map(\.id), ["w2"])
        XCTAssertEqual(FlashcardDeck.cards(words, toggle: "learned", learned: nil).map(\.id), ["w1", "w2", "w3"])
        // 保存待ちの「覚えた」も外れる
        XCTAssertEqual(FlashcardDeck.cards(words, toggle: "learned", learned: false, pending: ["w1": true]).map(\.id), ["w3"])
        XCTAssertTrue(FlashcardDeck.isLearned(Record(id: "x", data: ["learned": .string("true")]), toggle: "learned"))
        XCTAssertTrue(FlashcardDeck.isLearned(Record(id: "x", data: ["learned": .number(1)]), toggle: "learned"))
    }

    func testFlip() {
        var deck = FlashcardDeck()
        XCTAssertFalse(deck.flipped)
        deck.flip()
        XCTAssertTrue(deck.flipped)
        deck.flip()
        XCTAssertFalse(deck.flipped)
    }

    /// 覚えた → toggle に true の差分・次のカード・表に戻る。まだ → false。
    func testAnswerReturnsPatchAndAdvances() {
        var deck = FlashcardDeck()
        let cards = FlashcardDeck.cards(words, toggle: "learned", learned: nil)
        XCTAssertEqual(deck.current(in: cards)?.id, "w1")
        deck.flip()
        XCTAssertEqual(deck.answer(cards[0], learned: true, toggle: "learned"), ["learned": .bool(true)])
        XCTAssertFalse(deck.flipped, "次のカードは表から")
        XCTAssertEqual(deck.pending, ["w1": true])
        XCTAssertEqual(deck.current(in: cards)?.id, "w2")
        XCTAssertEqual(deck.answer(cards[1], learned: false, toggle: "learned"), ["learned": .bool(false)])
        XCTAssertEqual(deck.pending, ["w1": true, "w2": false])
        deck.settle("w1", requested: true)
        deck.settle("w2", requested: true) // 別の値を頼んだ結果は消さない
        XCTAssertEqual(deck.pending, ["w2": false])
        // toggle 無し = めくるだけ(保存しない)
        XCTAssertNil(deck.answer(cards[2], learned: true, toggle: nil))
        XCTAssertNil(deck.pending["w3"])
    }

    /// 覚えて山札から外れても「n / 全体」の全体は変わらない。全部答えたら完了・もう一度で最初から(覚えたものは出ない)。
    func testProgressDoneAndRestart() {
        var store = words
        var deck = FlashcardDeck()
        func cards() -> [Record] { FlashcardDeck.cards(store, toggle: "learned", learned: false, pending: deck.pending) }
        XCTAssertEqual(deck.progress(in: cards()).position, 1)
        XCTAssertEqual(deck.progress(in: cards()).total, 2)

        let first = try! XCTUnwrap(deck.current(in: cards()))
        XCTAssertEqual(first.id, "w1")
        deck.answer(first, learned: true, toggle: "learned")
        XCTAssertFalse(cards().contains { $0.id == "w1" }, "覚えたカードは山札から外れる")
        XCTAssertEqual(deck.progress(in: cards()).position, 2)
        XCTAssertEqual(deck.progress(in: cards()).total, 2)
        // 保存が終わってストアに反映
        store[1].data["learned"] = .bool(true)
        deck.settle("w1", requested: true)
        XCTAssertFalse(cards().contains { $0.id == "w1" })

        let second = try! XCTUnwrap(deck.current(in: cards()))
        XCTAssertEqual(second.id, "w3")
        XCTAssertFalse(deck.isDone(in: cards()))
        deck.answer(second, learned: false, toggle: "learned")
        XCTAssertNil(deck.current(in: cards()))
        XCTAssertTrue(deck.isDone(in: cards()))
        XCTAssertEqual(deck.progress(in: cards()).position, 2)

        deck.restart()
        XCTAssertFalse(deck.isDone(in: cards()))
        XCTAssertEqual(cards().map(\.id), ["w3"], "もう一度は覚えていないカードだけ")
        XCTAssertEqual(deck.progress(in: cards()).total, 1)
    }

    /// 最初から 0 枚(全部覚えた・未登録)は完了ではなく空の表示。
    func testEmptyDeck() {
        let deck = FlashcardDeck()
        let cards = FlashcardDeck.cards([words[2]], toggle: "learned", learned: false)
        XCTAssertTrue(cards.isEmpty)
        XCTAssertNil(deck.current(in: cards))
        XCTAssertFalse(deck.isDone(in: cards))
        XCTAssertEqual(deck.progress(in: cards).total, 0)
    }
}
