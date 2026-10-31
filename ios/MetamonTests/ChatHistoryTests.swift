import XCTest
import MetamonDSL
import MetamonServices
@testable import Metamon

/// アプリごとのチャット履歴(#210)。保存と復元・上限件数で切り捨て・反映の区切り・AI に送る文脈・削除で消えること。
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

    /// generate を「何も変わらなかった」(#211)にできるモック
    final class UnchangedAI: AIClient, @unchecked Sendable {
        let base = MockAIClient(delay: .milliseconds(10))
        private let lock = NSLock()
        private var _unchanged = false
        var unchanged: Bool {
            get { lock.withLock { _unchanged } }
            set { lock.withLock { _unchanged = newValue } }
        }

        func morph(transcript: String, appId: String, currentVersion: Int) async throws -> MorphResult {
            var result = try await base.morph(transcript: transcript, appId: appId, currentVersion: currentVersion)
            if unchanged { result.unchanged = true; result.summary = "変更できませんでした" }
            return result
        }
        func chat(appId: String, messages: [ChatMessage], locale: String?, mode: ChatMode?) async throws -> ChatTurn {
            try await base.chat(appId: appId, messages: messages, locale: locale, mode: mode)
        }
        func resetChat(appId: String) async {}
    }

    private func makeEditRuntime(ai: AIClient? = nil) throws -> AppRuntime {
        var services = BuildConfig.make(environment: .mock)
        if let ai { services.ai = ai }
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

    func testStoreKeepsLatestLimit() {
        let total = ChatHistoryStore.limit + 15
        let entries = (1...total).map { ChatHistoryEntry(kind: .user, text: "m\($0)") }
        store.save(entries, appId: "a")
        let loaded = store.load(appId: "a")
        XCTAssertEqual(loaded.count, ChatHistoryStore.limit)
        XCTAssertEqual(loaded.first?.text, "m16")
        XCTAssertEqual(loaded.last?.text, "m\(total)")
    }

    // MARK: - 回帰: 指示を重ねても履歴が消えない

    func testManyAppliesKeepFirstConversation() async throws {
        let rt = try makeEditRuntime()
        for i in 1...12 {
            await rt.submitUtterance("修正\(i)をして")
            await rt.buildFromChat()
            XCTAssertEqual(rt.document.version, i + 1)
        }
        XCTAssertEqual(rt.chatHistory.first?.text, "修正1をして", "修正を 12 回重ねても最初の会話が残る(30 件上限で押し出されていた)")
        XCTAssertEqual(rt.chatHistory.filter { $0.kind == .applied }.count, 12)
        XCTAssertEqual(store.load(appId: rt.document.appId), rt.chatHistory)
    }

    func testUnchangedReopenDoesNotDuplicateConversation() async throws {
        let ai = UnchangedAI()
        let rt = try makeEditRuntime(ai: ai)
        await rt.submitUtterance("金額を大きくして")
        await rt.buildFromChat()
        let before = rt.chatHistory
        XCTAssertEqual(before.map(\.kind), [.user, .assistant, .applied])

        ai.unchanged = true
        await rt.submitUtterance("見やすくして")
        for _ in 1...3 { // 何も変わらない → 開き直し、を繰り返す
            await rt.buildFromChat()
            XCTAssertTrue(rt.showVoiceSheet)
            XCTAssertEqual(rt.chatHistory, before, "開き直した会話を閉じた履歴に二重に残さない")
        }
        XCTAssertEqual(rt.chatBubbles.map(\.role), [.user, .assistant, .assistant, .assistant, .assistant])
        XCTAssertEqual(store.load(appId: rt.document.appId).count, before.count + rt.chatBubbles.count)

        ai.unchanged = false
        await rt.buildFromChat()
        XCTAssertEqual(rt.chatHistory.count, before.count + 5 + 1, "反映で今の会話が 1 回だけ移る + 区切り")
        XCTAssertEqual(rt.chatHistory.filter { $0.text == "見やすくして" }.count, 1)
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
        XCTAssertTrue(ChatContext.dividerText(divider).contains("v2"))
        XCTAssertTrue(ChatContext.dividerText(divider).hasSuffix("・" + divider.text))

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
        XCTAssertTrue(messages[0].text.hasPrefix(L10n.tr("(これまでに反映した変更)") + "\n"))
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
