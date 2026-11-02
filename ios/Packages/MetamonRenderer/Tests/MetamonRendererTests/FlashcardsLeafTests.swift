import XCTest
import SwiftUI
import Observation
import MetamonDSL
@testable import MetamonRenderer

/// 保存(`updateRecord`)をストアにそのまま反映するランタイム。
@MainActor
@Observable
private final class CardsRuntime: DSLRuntime {
    var dsl: AppDSL
    var paths: [String: [Route]] = [:]
    var selectedTab = ""
    var loading: Set<String> = []
    var store: [String: [Record]]
    var updates: [(entity: String, recordId: String, data: [String: JSONValue])] = []

    init(dsl: AppDSL, store: [String: [Record]]) {
        self.dsl = dsl
        self.store = store
    }

    func records(for entity: String) -> [Record] { store[entity] ?? [] }
    func load(entities: Set<String>, force: Bool) async {}
    func run(_ action: Action, _ ctx: ActionContext) async {}
    func close(_ route: Route) {}
    func pathBinding(_ stack: String) -> Binding<[Route]> { .constant([]) }
    func updateRecord(entity: String, recordId: String, data: [String: JSONValue]) async -> Bool {
        updates.append((entity, recordId, data))
        guard let i = store[entity]?.firstIndex(where: { $0.id == recordId }) else { return false }
        for (k, v) in data { store[entity]![i].data[k] = v }
        return true
    }
}

/// `flashcards`(#264): 登録・山札(filter は値の無いカードを「まだ」)・覚えた / まだ が record の更新になり、覚えたカードが山札から外れる。
final class FlashcardsLeafTests: XCTestCase {
    private func vocab() throws -> AppDSL {
        var url = URL(fileURLWithPath: #filePath)
        for _ in 0..<6 { url.deleteLastPathComponent() }
        return try AppDSL.load(contentsOf: url.appending(path: "docs/dsl/examples/vocab.json"))
    }

    private let words = [
        Record(id: "w1", data: ["term": "apple", "meaning": "りんご", "learned": .bool(false)], createdAt: "2026-11-01T00:00:00Z"),
        Record(id: "w2", data: ["term": "borrow", "meaning": "借りる", "learned": .bool(true)], createdAt: "2026-11-02T00:00:00Z"),
        Record(id: "w3", data: ["term": "consider", "meaning": "よく考える"], createdAt: "2026-11-03T00:00:00Z"),
    ]

    private func component(_ dsl: AppDSL) throws -> Component {
        try XCTUnwrap(dsl.screens.flatMap(\.allComponents).first { $0.type == .flashcards })
    }

    @MainActor func testRegisteredInEveryBuild() throws {
        XCTAssertNotNil(ComponentRegistry.standard.builder(for: "flashcards"))
        XCTAssertFalse(ClientCapabilities.reviewExcludedComponents.contains(.flashcards), "OS の機能を使わないので審査用ビルドにも入れる")
        XCTAssertTrue(try component(vocab()).rendersOwnSurface)
    }

    @MainActor func testDeckShowsOnlyUnlearnedInOrder() throws {
        let dsl = try vocab()
        let rt = CardsRuntime(dsl: dsl, store: ["word": words])
        let c = try component(dsl)
        XCTAssertEqual(FlashcardsLeaf.learnedFilter(c), false)
        XCTAssertEqual(FlashcardsLeaf.toggleField(c, in: dsl.entity("word")), "learned")
        // learned の無い w3 も「まだ」で出る(ランタイムの filter だけなら eq false に一致しない)
        XCTAssertEqual(FlashcardsLeaf.cards(c, rt: rt, pending: [:]).map(\.id), ["w1", "w3"])
    }

    /// Leaf の answer と同じ流れ: 覚えた → updateRecord(learned: true)→ 山札から外れる。まだ → false で残る。
    @MainActor func testMarkLearnedUpdatesRecordAndDropsCard() async throws {
        let dsl = try vocab()
        let rt = CardsRuntime(dsl: dsl, store: ["word": words])
        let c = try component(dsl)
        let toggle = FlashcardsLeaf.toggleField(c, in: dsl.entity("word"))
        var deck = FlashcardDeck()

        let first = try XCTUnwrap(deck.current(in: FlashcardsLeaf.cards(c, rt: rt, pending: deck.pending)))
        let patch = try XCTUnwrap(deck.answer(first, learned: true, toggle: toggle))
        XCTAssertFalse(FlashcardsLeaf.cards(c, rt: rt, pending: deck.pending).contains { $0.id == "w1" }, "保存の前から外れる(楽観的)")
        let saved = await rt.updateRecord(entity: "word", recordId: first.id, data: patch)
        XCTAssertTrue(saved)
        deck.settle(first.id, requested: true)
        XCTAssertEqual(rt.updates.last?.recordId, "w1")
        XCTAssertEqual(rt.updates.last?.data, ["learned": .bool(true)])
        XCTAssertEqual(FlashcardsLeaf.cards(c, rt: rt, pending: deck.pending).map(\.id), ["w3"])

        let second = try XCTUnwrap(deck.current(in: FlashcardsLeaf.cards(c, rt: rt, pending: deck.pending)))
        XCTAssertEqual(second.id, "w3")
        let notYet = try XCTUnwrap(deck.answer(second, learned: false, toggle: toggle))
        _ = await rt.updateRecord(entity: "word", recordId: second.id, data: notYet)
        deck.settle(second.id, requested: false)
        XCTAssertEqual(rt.updates.last?.data, ["learned": .bool(false)])
        let cards = FlashcardsLeaf.cards(c, rt: rt, pending: deck.pending)
        XCTAssertEqual(cards.map(\.id), ["w3"], "まだのカードは山札に残る")
        XCTAssertTrue(deck.isDone(in: cards), "この回は全部答えた")
        XCTAssertEqual(deck.progress(in: cards).total, 2)
    }

    /// toggle が boolean でない(または無い)時は記録せずにめくるだけ。
    @MainActor func testNoToggleOnlyFlips() throws {
        let json = #"{"type":"flashcards","entity":"word","front":"term","back":"meaning","toggle":"term"}"#
        let c = try JSONDecoder().decode(Component.self, from: Data(json.utf8))
        let dsl = try vocab()
        XCTAssertNil(FlashcardsLeaf.toggleField(c, in: dsl.entity("word")))
        var deck = FlashcardDeck()
        XCTAssertNil(deck.answer(words[0], learned: true, toggle: FlashcardsLeaf.toggleField(c, in: dsl.entity("word"))))
    }
}
