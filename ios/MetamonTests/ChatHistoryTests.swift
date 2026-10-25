import XCTest
import MetamonDSL
import MetamonServices
@testable import Metamon

/// アプリごとのチャット履歴(#210)。保存と復元・30 件で切り捨て・反映の区切り・AI に送る文脈・削除で消えること。
@MainActor
final class ChatHistoryTests: XCTestCase {
    private var suiteName = ""
    private var store: ChatHistoryStore!

    override func setUp() {
        suiteName = "ChatHistoryTests.\(UUID().uuidString)"
        store = ChatHistoryStore(defaults: UserDefaults(suiteName: suiteName)!)
    }

    override func tearDown() {
        UserDefaults().removePersistentDomain(forName: suiteName)
    }

    private func makeEditRuntime() throws -> AppRuntime {
        let services = BuildConfig.make(environment: .mock)
        let doc = try DSLExamples.document("kakeibo", version: 1)
        let rt = AppRuntime(document: doc, services: services)
        rt.clarifyEnabled = true
        rt.chatHistoryStore = store
        return rt
    }

    // MARK: - 保存

    func testStoreRoundTripPerApp() {
        let a = [ChatHistoryEntry(kind: .user, text: "金額を大きくして"), ChatHistoryEntry(kind: .applied, text: "金額を大きくしました", version: 2)]
        store.save(a, appId: "a")
        store.save([ChatHistoryEntry(kind: .user, text: "別のアプリ")], appId: "b")
        XCTAssertEqual(store.load(appId: "a"), a)
        XCTAssertEqual(store.load(appId: "b").map(\.text), ["別のアプリ"])
        XCTAssertEqual(store.load(appId: "c"), [])
    }

    func testStoreKeepsLatest30() {
        let entries = (1...45).map { ChatHistoryEntry(kind: .user, text: "m\($0)") }
        store.save(entries, appId: "a")
        let loaded = store.load(appId: "a")
        XCTAssertEqual(loaded.count, ChatHistoryStore.limit)
        XCTAssertEqual(loaded.first?.text, "m16")
        XCTAssertEqual(loaded.last?.text, "m45")
    }

    // MARK: - 反映の区切りと復元

    func testApplyLeavesHistoryWithDividerAndRestores() async throws {
        let rt = try makeEditRuntime()
        await rt.submitUtterance("金額を大きくして")
        XCTAssertEqual(rt.chatTurn?.kind, .ready)
        await rt.buildFromChat()
        XCTAssertEqual(rt.document.version, 2)
        XCTAssertFalse(rt.isChatting, "反映で今の会話は閉じる")
        XCTAssertEqual(rt.chatHistory.map(\.kind), [.user, .assistant, .applied], "閉じた会話と区切りが残る")
        let divider = try XCTUnwrap(rt.chatHistory.last)
        XCTAssertEqual(divider.version, 2)
        XCTAssertEqual(divider.text, "「金額を大きくして」を反映しました(モック)", "要約は generate の summary")
        XCTAssertTrue(ChatContext.dividerText(divider).hasPrefix("ここで反映しました(v2)"))

        // 再起動相当: 同じ保存先から別の AppRuntime で読み直す
        let reopened = try makeEditRuntime()
        XCTAssertEqual(reopened.chatHistory, rt.chatHistory)
    }

    func testLiveChatIsSavedEveryBubble() async throws {
        let rt = try makeEditRuntime()
        await rt.submitUtterance("見やすくして")
        XCTAssertEqual(store.load(appId: rt.document.appId).map(\.kind), [.user, .assistant], "反映前でも吹き出しごとに保存(再起動しても残る)")
    }

    func testRestartClearsHistory() async throws {
        let rt = try makeEditRuntime()
        await rt.submitUtterance("金額を大きくして")
        await rt.buildFromChat()
        XCTAssertTrue(rt.hasChatHistory)
        rt.restartChat()
        XCTAssertEqual(rt.chatHistory, [])
        XCTAssertEqual(store.load(appId: rt.document.appId), [])
    }

    // MARK: - AI に送る文脈

    func testContextIsSummariesPlusSinceLastApply() {
        var history: [ChatHistoryEntry] = []
        for v in 2...8 {
            history.append(ChatHistoryEntry(kind: .user, text: "古い指示\(v)"))
            history.append(ChatHistoryEntry(kind: .assistant, text: "古い答え\(v)"))
            history.append(ChatHistoryEntry(kind: .applied, text: "変更\(v)", version: v))
        }
        history.append(ChatHistoryEntry(kind: .user, text: "未反映の指示"))
        let messages = ChatContext.messages(history: history, live: [ChatMessage(role: .user, text: "今の指示")])
        XCTAssertEqual(messages.count, 3, "要約 1 + 最後の反映以降 1 + 今の会話 1")
        XCTAssertEqual(messages[0].role, .assistant)
        XCTAssertTrue(messages[0].text.hasPrefix("(これまでに反映した変更)"))
        XCTAssertEqual(messages[0].text.split(separator: "\n").count, 1 + ChatContext.maxSummaries, "要約は最大 5 件")
        XCTAssertTrue(messages[0].text.contains("v4: 変更4") && messages[0].text.contains("v8: 変更8"))
        XCTAssertFalse(messages[0].text.contains("v3:"))
        XCTAssertFalse(messages.contains { $0.text.contains("古い指示") }, "反映済みの古いやり取りは送らない")
        XCTAssertEqual(messages[1], ChatMessage(role: .user, text: "未反映の指示"))
        XCTAssertEqual(messages[2], ChatMessage(role: .user, text: "今の指示"))
    }

    func testContextWithoutHistoryIsLiveOnly() {
        let live = [ChatMessage(role: .user, text: "金額を大きくして")]
        XCTAssertEqual(ChatContext.messages(history: [], live: live), live)
    }

    func testRuntimeSendsSummaryAfterApply() async throws {
        let rt = try makeEditRuntime()
        await rt.submitUtterance("金額を大きくして")
        await rt.buildFromChat()
        await rt.submitUtterance("見やすくして")
        let sent = rt.chatContextMessages
        XCTAssertEqual(sent.map(\.role), [.assistant, .user, .assistant], "要約 + 今の会話だけ")
        XCTAssertTrue(sent[0].text.contains("v2: 「金額を大きくして」を反映しました(モック)"))
        XCTAssertEqual(sent[1].text, "見やすくして")
        // 画面には前の会話も区切りも見えている
        XCTAssertEqual(rt.chatHistory.map(\.kind), [.user, .assistant, .applied])
        XCTAssertEqual(rt.chatBubbles.map(\.role), [.user, .assistant])
    }

    // MARK: - 削除

    func testDeleteAppRemovesHistory() async throws {
        let rt = try makeEditRuntime()
        store.save([ChatHistoryEntry(kind: .user, text: "消えるはず")], appId: "other")
        await rt.deleteApp(AppSummary(id: "other", name: "別アプリ", currentVersion: 1))
        XCTAssertEqual(store.load(appId: "other"), [])
    }

    func testRemoveAllClearsEveryApp() {
        store.save([ChatHistoryEntry(kind: .user, text: "a")], appId: "a")
        store.save([ChatHistoryEntry(kind: .user, text: "b")], appId: "b")
        store.removeAll()
        XCTAssertEqual(store.load(appId: "a"), [])
        XCTAssertEqual(store.load(appId: "b"), [])
    }
}
