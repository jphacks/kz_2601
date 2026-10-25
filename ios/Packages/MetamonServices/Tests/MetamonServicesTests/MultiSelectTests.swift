import XCTest
@testable import MetamonServices

/// #220 複数選択: デコード・instruction のつなぎ方・0 件で押せない・モック。
final class MultiSelectTests: XCTestCase {
    func testDecodesMultiSelectProposal() throws {
        let json = #"{"kind":"proposal","text":"案","turn":1,"multiSelect":true,"options":[{"id":"opt1","title":"グラフ","detail":"","instruction":"円グラフを追加して"},{"id":"opt2","title":"予算","detail":"","instruction":"予算バーを追加して"}]}"#
        let t = try JSONDecoder().decode(ChatTurn.self, from: Data(json.utf8))
        XCTAssertEqual(t.multiSelect, true)
        XCTAssertTrue(t.isMultiSelect)
    }

    func testMissingMultiSelectIsSingle() throws {
        let json = #"{"kind":"question","text":"どの単位で集計しますか?","turn":1,"suggestions":["日","週"]}"#
        let t = try JSONDecoder().decode(ChatTurn.self, from: Data(json.utf8))
        XCTAssertNil(t.multiSelect, "旧 backend は送らない")
        XCTAssertFalse(t.isMultiSelect)
    }

    func testJoinedInstruction() {
        XCTAssertEqual(MultiSelect.joinedInstruction(["円グラフを追加して"]), "円グラフを追加して", "1 件はそのまま")
        XCTAssertEqual(MultiSelect.joinedInstruction([" 円グラフを追加して ", "", "予算バーを追加して"]),
                       "・円グラフを追加して\n・予算バーを追加して")
        XCTAssertEqual(MultiSelect.joinedInstruction([]), "")
    }

    func testJoinedAnswer() {
        XCTAssertEqual(MultiSelect.joinedAnswer(["金額", "メモ"]), "金額、メモ")
    }

    func testCannotSubmitWithZero() {
        XCTAssertFalse(MultiSelect.canSubmit(selectedCount: 0))
        XCTAssertTrue(MultiSelect.canSubmit(selectedCount: 1))
        XCTAssertNil(MultiSelect.combined([]))
    }

    func testCombinedKeepsOrderAndPaidFeatures() throws {
        let paid = PaidFeature(id: "sharedData", title: "共有")
        let a = ChatOption(id: "opt1", title: "グラフ", instruction: "円グラフを追加して")
        let b = ChatOption(id: "opt2", title: "共有", instruction: "みんなで共有して", requiresPro: true, paidFeatures: [paid],
                           freeInstruction: "自分だけで使う")
        let c = try XCTUnwrap(MultiSelect.combined([a, b]))
        XCTAssertEqual(c.id, "opt1+opt2")
        XCTAssertEqual(c.instruction, "・円グラフを追加して\n・みんなで共有して")
        XCTAssertEqual(c.requiresPro, true)
        XCTAssertEqual(c.paidFeatures?.map(\.id), ["sharedData"])
        XCTAssertEqual(c.freeInstruction, "・円グラフを追加して\n・自分だけで使う")
        let free = try XCTUnwrap(MultiSelect.combined([a, a]))
        XCTAssertNil(free.requiresPro)
        XCTAssertEqual(MultiSelect.combined([a]), a)
    }

    func testMockReturnsMultiSelect() async throws {
        let mock = MockAIClient(delay: .zero)
        let p = try await mock.chat(appId: "a", messages: [.init(role: .user, text: "機能を足したい")], locale: "ja", mode: .edit)
        XCTAssertEqual(p.kind, .proposal)
        XCTAssertTrue(p.isMultiSelect)
        XCTAssertEqual(p.options?.count, 3)
        let q = try await mock.chat(appId: "a", messages: [.init(role: .user, text: "項目を選びたい")], locale: "ja", mode: .create)
        XCTAssertEqual(q.kind, .question)
        XCTAssertTrue(q.isMultiSelect)
        let single = try await mock.chat(appId: "a", messages: [.init(role: .user, text: "見やすくして")], locale: "ja", mode: .edit)
        XCTAssertFalse(single.isMultiSelect)
    }
}
