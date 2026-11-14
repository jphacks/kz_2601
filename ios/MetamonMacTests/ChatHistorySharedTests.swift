import XCTest
import MetamonServices
@testable import Metamon

/// ios/Shared の会話の履歴(#210)のうち、Mac でもそのまま使う部分。上限での切り詰め・反映の区切りの要約の行・
/// `forRuntime` の保存先(テストでは実機の UserDefaults を汚さない)。全体実行で順番に頼らない(保存先は毎回新しく作る)
final class ChatHistorySharedTests: XCTestCase {
    private func entries(_ count: Int, from: Int = 1) -> [ChatHistoryEntry] {
        (from..<(from + count)).map { ChatHistoryEntry(kind: .user, text: "m\($0)") }
    }

    // MARK: - 上限での切り詰め

    func testTrimmedKeepsEverythingUpToLimit() {
        XCTAssertTrue(ChatHistoryStore.trimmed([]).isEmpty)
        let justUnder = entries(ChatHistoryStore.limit - 1)
        XCTAssertEqual(ChatHistoryStore.trimmed(justUnder), justUnder)
        let exact = entries(ChatHistoryStore.limit)
        XCTAssertEqual(ChatHistoryStore.trimmed(exact), exact, "ちょうど上限なら切らない")
    }

    func testTrimmedKeepsLatestWhenOverLimit() {
        let total = ChatHistoryStore.limit + 15
        let kept = ChatHistoryStore.trimmed(entries(total))
        XCTAssertEqual(kept.count, ChatHistoryStore.limit)
        XCTAssertEqual(kept.first?.text, "m16", "古い方から押し出す")
        XCTAssertEqual(kept.last?.text, "m\(total)")
    }

    func testTrimmedCountsDividersAsOneEntry() {
        var history = entries(ChatHistoryStore.limit)
        history.append(ChatHistoryEntry(kind: .applied, text: "反映しました", version: 3))
        let kept = ChatHistoryStore.trimmed(history)
        XCTAssertEqual(kept.count, ChatHistoryStore.limit)
        XCTAssertEqual(kept.last?.kind, .applied, "区切りも 1 件と数える")
        XCTAssertEqual(kept.first?.text, "m2")
    }

    func testSaveAndLoadApplyTheSameLimit() {
        let store = ChatHistoryStore()
        let total = ChatHistoryStore.limit + 5
        store.save(entries(total), appId: "mac-shared")
        let loaded = store.load(appId: "mac-shared")
        XCTAssertEqual(loaded.count, ChatHistoryStore.limit)
        XCTAssertEqual(loaded.last?.text, "m\(total)")
        XCTAssertEqual(store.load(appId: "ほかのアプリ"), [], "ほかのアプリの履歴は混ざらない")
    }

    /// 読み直した時に、吹き出しも区切りも揃った「閉じた会話」として戻る
    func testLoadRestoresClosedConversationWithDividers() {
        let store = ChatHistoryStore()
        let history = [ChatHistoryEntry(kind: .user, text: "金額を大きくして"),
                       ChatHistoryEntry(kind: .assistant, text: "大きくしますか?"),
                       ChatHistoryEntry(kind: .applied, text: "金額を大きくしました", version: 2)]
        store.save(history, appId: "mac-shared")
        XCTAssertEqual(store.load(appId: "mac-shared"), history)
        store.save([], appId: "mac-shared")
        XCTAssertEqual(store.load(appId: "mac-shared"), [], "空を保存したら消す")
    }

    // MARK: - 反映の区切りの要約の行

    func testSummaryLineJoinsVersionAndText() {
        XCTAssertEqual(ChatContext.summaryLine(.init(kind: .applied, text: "金額を大きくしました", version: 2)),
                       "v2: 金額を大きくしました")
        XCTAssertEqual(ChatContext.summaryLine(.init(kind: .applied, text: "  色を変えました\n", version: 10)),
                       "v10: 色を変えました", "前後の空白は落とす")
    }

    func testSummaryLineWithOnlyOneSide() {
        XCTAssertEqual(ChatContext.summaryLine(.init(kind: .applied, text: "作りました")), "作りました", "版が無ければ要約だけ")
        XCTAssertEqual(ChatContext.summaryLine(.init(kind: .applied, text: "   ", version: 4)), "v4", "要約が無ければ版だけ")
        XCTAssertEqual(ChatContext.summaryLine(.init(kind: .applied, text: "")), "", "どちらも無ければ空")
    }

    func testSummaryLineIsWhatTheContextSends() {
        let divider = ChatHistoryEntry(kind: .applied, text: "青にしました", version: 3)
        let messages = ChatContext.messages(history: [divider], live: [])
        XCTAssertEqual(messages.map(\.role), [.assistant], "要約は assistant の 1 通")
        XCTAssertTrue(messages[0].text.hasSuffix("- " + ChatContext.summaryLine(divider)))
    }

    // MARK: - forRuntime の保存先

    func testForRuntimeStaysInMemoryDuringTests() {
        let appId = "mac-shared.\(UUID().uuidString)"
        let store = ChatHistoryStore.forRuntime()
        XCTAssertFalse(store === ChatHistoryStore.shared, "テストでは実機と同じ保存先を使わない")
        store.save([ChatHistoryEntry(kind: .user, text: "残らないはず")], appId: appId)
        XCTAssertNil(UserDefaults.standard.data(forKey: ChatHistoryStore.keyPrefix + appId), "UserDefaults を汚さない")
        XCTAssertEqual(ChatHistoryStore.shared.load(appId: appId), [])
    }

    func testForRuntimeGivesEachRuntimeItsOwnStore() {
        let appId = "mac-shared.\(UUID().uuidString)"
        let first = ChatHistoryStore.forRuntime()
        first.save([ChatHistoryEntry(kind: .user, text: "前のテストの履歴")], appId: appId)
        XCTAssertEqual(first.load(appId: appId).map(\.text), ["前のテストの履歴"])
        XCTAssertEqual(ChatHistoryStore.forRuntime().load(appId: appId), [], "別のテストの履歴を持ち込まない")
    }
}
