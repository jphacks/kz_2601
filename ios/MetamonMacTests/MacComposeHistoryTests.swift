import XCTest
import MetamonDSL
import MetamonServices
@testable import Metamon

/// 会話のシートの履歴・あいさつ・「質問 n / 3」・今日の残り(#1211)
@MainActor
final class MacComposeHistoryTests: XCTestCase {
    private let appId = "mac-history"

    private func editSession(_ store: ChatHistoryStore, ai: any AIClient) -> MacComposeSession {
        MacComposeSession(target: .edit(appId: appId, name: "家計簿", version: 2), ai: ai, reduceMotion: { true },
                          isConsentGranted: { true }, clarifyEnabled: true, clarifyAutoBuild: false, historyStore: store)
    }

    private func waitUntil(_ condition: () -> Bool) async throws {
        for _ in 0..<150 where !condition() { try await Task.sleep(for: .milliseconds(20)) }
    }

    private func document(version: Int) throws -> AppDocument {
        let url = try XCTUnwrap(Bundle(for: MacAppRuntime.self).url(forResource: "kakeibo", withExtension: "json"))
        return AppDocument(appId: appId, version: version, dsl: try AppDSL.load(contentsOf: url))
    }

    // MARK: - 履歴

    func testHistoryPersistsAndRestoresInNextSheet() async throws {
        let store = ChatHistoryStore()
        let ai = ScriptedAI(turns: [ChatTurn(kind: .question, text: "どの色にしますか?", turn: 1)])
        let first = editSession(store, ai: ai)
        XCTAssertFalse(first.hasHistory)
        first.send("色を変えたい")
        try await waitUntil { first.bubbles.count == 2 }
        first.close()

        let second = editSession(store, ai: ai)
        XCTAssertTrue(second.bubbles.isEmpty, "開き直すと前の会話は閉じた会話になる")
        XCTAssertEqual(second.history.map(\.kind), [.user, .assistant])
        XCTAssertEqual(second.history.map(\.text), ["色を変えたい", "どの色にしますか?"])
        XCTAssertEqual(second.contextMessages.map(\.text), ["色を変えたい", "どの色にしますか?"], "続きから AI に送る")
    }

    func testStoreRoundTripsThroughUserDefaultsAndRemoveAll() throws {
        let suite = "metamon.mac-history-test.\(UUID().uuidString)"
        let defaults = try XCTUnwrap(UserDefaults(suiteName: suite))
        defer { defaults.removePersistentDomain(forName: suite) }
        let entries = [ChatHistoryEntry(kind: .user, text: "家計簿"), ChatHistoryEntry(kind: .applied, text: "作りました", version: 1)]
        ChatHistoryStore(defaults: defaults).save(entries, appId: appId)
        XCTAssertEqual(ChatHistoryStore(defaults: defaults).load(appId: appId), entries, "別のインスタンス(次の起動)でも読める")
        ChatHistoryStore(defaults: defaults).removeAll()
        XCTAssertTrue(ChatHistoryStore(defaults: defaults).load(appId: appId).isEmpty)
    }

    func testAppliedBuildAddsDividerWithVersion() async throws {
        let store = ChatHistoryStore()
        let ai = ScriptedAI(turns: [ChatTurn(kind: .question, text: "どの色?", turn: 1)],
                            result: MorphResult(document: try document(version: 3), summary: "青にしました"))
        let session = editSession(store, ai: ai)
        session.send("青にして")
        try await waitUntil { session.bubbles.count == 2 }
        session.build("青にして")
        try await waitUntil { if case .finished = session.phase { true } else { false } }
        XCTAssertEqual(session.history.last?.kind, .applied)
        XCTAssertEqual(session.history.last?.version, 3)
        XCTAssertEqual(store.load(appId: appId).last.map(ChatContext.dividerText), ChatContext.dividerText(.init(kind: .applied, text: "青にしました", version: 3)))
        XCTAssertTrue(session.bubbles.isEmpty, "反映で今の会話を閉じる")
    }

    func testRestartLiveKeepsHistoryAndRestartAllClearsIt() async throws {
        let store = ChatHistoryStore()
        store.save([ChatHistoryEntry(kind: .user, text: "前の会話")], appId: appId)
        let ai = ScriptedAI(turns: [ChatTurn(kind: .question, text: "どこを?", turn: 2)])
        let session = editSession(store, ai: ai)
        session.send("続き")
        try await waitUntil { session.bubbles.count == 2 }

        session.requestRestart()
        XCTAssertTrue(session.showsRestartConfirm, "これまでの会話があれば確認する")
        session.restartLive()
        XCTAssertTrue(session.bubbles.isEmpty)
        XCTAssertEqual(store.load(appId: appId).map(\.text), ["前の会話"])

        session.restartAll()
        XCTAssertFalse(session.hasHistory)
        XCTAssertTrue(store.load(appId: appId).isEmpty)
    }

    func testClosedSessionDoesNotWriteHistoryBack() async throws {
        let store = ChatHistoryStore()
        let ai = ScriptedAI(turns: [ChatTurn(kind: .question, text: "どの色?", turn: 1)])
        let session = editSession(store, ai: ai)
        session.send("色")
        try await waitUntil { session.bubbles.count == 2 }
        session.close()
        store.removeAll() // サインアウトの後始末
        session.restartLive() // 閉じた後に履歴を書こうとする
        XCTAssertTrue(store.load(appId: appId).isEmpty, "閉じた会話は消した履歴を書き戻さない")
    }

    func testWipeStopsOpenSessionFromWritingHistoryBack() async throws {
        let store = ChatHistoryStore()
        let ai = ScriptedAI(turns: [ChatTurn(kind: .question, text: "どの色?", turn: 1)])
        let session = editSession(store, ai: ai)
        session.send("色")
        try await waitUntil { session.bubbles.count == 2 }
        MacComposeSession.wipeCount += 1 // `MacLocalWipe.run` が進める
        store.removeAll()
        session.restartLive()
        XCTAssertTrue(store.load(appId: appId).isEmpty, "後始末の後は閉じていない会話も書き戻さない")
        let next = editSession(store, ai: ai)
        next.restartLive()
        XCTAssertTrue(next.history.isEmpty)
    }

    func testDeletingAppClearsItsHistory() throws {
        let store = ChatHistoryStore()
        store.save([ChatHistoryEntry(kind: .user, text: "家計簿")], appId: appId)
        store.save([ChatHistoryEntry(kind: .user, text: "ほかのアプリ")], appId: "other")
        let library = MacAppLibrary(store: nil, chatHistory: store)
        library.apply(try document(version: 1), created: true)
        library.delete(appId)
        XCTAssertTrue(store.load(appId: appId).isEmpty)
        XCTAssertEqual(store.load(appId: "other").count, 1, "ほかのアプリの履歴は残す")
    }

    // MARK: - 質問 n / 3

    func testQuestionProgressCountsCreateQuestions() async throws {
        let ai = ScriptedAI(turns: [ChatTurn(kind: .question, text: "何を記録?", turn: 1),
                                    ChatTurn(kind: .question, text: "単位は?", turn: 2),
                                    ChatTurn(kind: .ready, text: "まとめました", turn: 3)])
        let session = MacComposeSession(target: .create, ai: ai, reduceMotion: { true }, isConsentGranted: { true },
                                        clarifyEnabled: true, clarifyAutoBuild: false, historyStore: ChatHistoryStore())
        XCTAssertNil(session.questionNumber)
        session.send("家計簿")
        try await waitUntil { session.bubbles.count == 2 }
        XCTAssertEqual(session.questionNumber, 1)
        session.send("支出")
        try await waitUntil { session.bubbles.count == 4 }
        XCTAssertEqual(session.questionNumber, 2)
        let label = QuestionProgress.label(2)
        XCTAssertTrue(label.contains("2") && label.contains("3"), label)
        XCTAssertNotEqual(label, "質問 %lld / %lld", "Mac の文言にも入っている")
        session.send("円")
        try await waitUntil { session.bubbles.count == 6 }
        XCTAssertNil(session.questionNumber, "案・要約では出さない")
        session.close()
    }

    func testQuestionProgressHiddenWhenEditing() async throws {
        let ai = ScriptedAI(turns: [ChatTurn(kind: .question, text: "どの色?", turn: 1)])
        let session = editSession(ChatHistoryStore(), ai: ai)
        session.send("色")
        try await waitUntil { session.bubbles.count == 2 }
        XCTAssertNil(session.questionNumber, "直す時は上限が無いので出さない")
    }

    // MARK: - あいさつ・今日の残り

    func testGreetingFollowsTimeOfDayAndVisit() throws {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = try XCTUnwrap(TimeZone(identifier: "Asia/Tokyo"))
        let morning = try XCTUnwrap(calendar.date(from: DateComponents(year: 2026, month: 11, day: 11, hour: 8)))
        let first = MacComposeGreeting.copy(for: .create, returning: false, date: morning, calendar: calendar)
        XCTAssertEqual(first.title, Greeting.copy(for: .firstVisit, date: morning, calendar: calendar).title)
        XCTAssertTrue(first.subtitle.hasPrefix(Greeting.timeOfDay(morning, calendar: calendar)))
        let again = MacComposeGreeting.copy(for: .create, returning: true, date: morning, calendar: calendar)
        XCTAssertEqual(again.title, Greeting.copy(for: .returning).title)
        let edit = MacComposeGreeting.copy(for: .edit(appId: appId, name: "家計簿", version: 1), returning: true, date: morning, calendar: calendar)
        XCTAssertEqual(edit.title, Greeting.copy(for: .editing).title)
    }

    func testRemainingTodayShownOnlyWhenKnown() {
        let now = Date()
        XCTAssertNil(MacRemainingTodayBadge.text(.of(nil, now: now, isPro: false, isAnonymous: false)), "分からない時は出さない")
        let known = RemainingTodayDisplay.of(RemainingToday(remaining: 20, observedAt: now), now: now, isPro: false, isAnonymous: false)
        XCTAssertEqual(MacRemainingTodayBadge.text(known), RemainingTodayDisplay.label(20))
        XCTAssertFalse(MacRemainingTodayBadge.isWarning(known))
        let low = RemainingTodayDisplay.of(RemainingToday(remaining: 2, observedAt: now), now: now, isPro: true, isAnonymous: false)
        XCTAssertEqual(MacRemainingTodayBadge.text(low), RemainingTodayDisplay.label(2))
        XCTAssertTrue(MacRemainingTodayBadge.isWarning(low))
        XCTAssertTrue(MacRemainingTodayBadge.isPro(low))
        let out = RemainingTodayDisplay.of(RemainingToday(remaining: 0, observedAt: now), now: now, isPro: false, isAnonymous: true)
        XCTAssertEqual(MacRemainingTodayBadge.text(out), RemainingTodayDisplay.label(0))
    }

    /// 決まった順に応答する AI(ネットにはつながない)
    private actor ScriptedAI: AIClient {
        var turns: [ChatTurn]
        let result: MorphResult?

        init(turns: [ChatTurn], result: MorphResult? = nil) {
            self.turns = turns
            self.result = result
        }

        func morph(transcript: String, appId: String, currentVersion: Int) async throws -> MorphResult {
            guard let result else { throw CancellationError() }
            return result
        }

        func chat(appId: String, messages: [ChatMessage], locale: String?, mode: ChatMode?) async throws -> ChatTurn {
            guard !turns.isEmpty else { throw CancellationError() }
            return turns.removeFirst()
        }

        func resetChat(appId: String) async {}
    }
}
