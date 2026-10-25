import XCTest
import MetamonDSL
import MetamonServices
@testable import Metamon

/// 有料機能の確認カード(#77)を AppRuntime + モック AI で確かめる。
/// 「これでいく」→ カード →「この機能なしで作る」で freeTranscript が generate に渡る / Pro・無効化中は出ない。
@MainActor
final class PaidFeatureConfirmTests: XCTestCase {
    /// generate に渡った文を記録するモック
    final class SpyAI: AIClient, @unchecked Sendable {
        let base = MockAIClient(delay: .milliseconds(50))
        private let lock = NSLock()
        private var _sent: [String] = []
        var sent: [String] { lock.withLock { _sent } }

        func morph(transcript: String, appId: String, currentVersion: Int) async throws -> MorphResult {
            lock.withLock { _sent.append(transcript) }
            return try await base.morph(transcript: transcript, appId: appId, currentVersion: currentVersion)
        }
        func chat(appId: String, messages: [ChatMessage], locale: String?, mode: ChatMode?) async throws -> ChatTurn {
            try await base.chat(appId: appId, messages: messages, locale: locale, mode: mode)
        }
        func resetChat(appId: String) async {}
    }

    private let sharedRequest = "サークルのみんなで予定を書き込める掲示板アプリを作って"

    private func makeRuntime(enforced: Bool, example: String = DSLExamples.initial) throws -> (AppRuntime, SpyAI) {
        let spy = SpyAI()
        var services = BuildConfig.make(environment: .mock)
        services.ai = spy
        let version = example == DSLExamples.initial ? 0 : 1
        let rt = AppRuntime(document: try DSLExamples.document(example, version: version), services: services)
        rt.clarifyEnabled = true
        rt.clarifyAutoBuild = false
        let defaults = UserDefaults(suiteName: "PaidFeatureConfirmTests.\(UUID().uuidString)")!
        rt.proGate = ProGate(store: ProEntitlementStore(), usage: CreationUsageStore(defaults: defaults),
                             quota: CreationQuota(dailyFreeLimit: 100), limitDisabled: !enforced)
        return (rt, spy)
    }

    func testReadyWithPaidFeatureShowsCardThenBuildsWithout() async throws {
        let (rt, spy) = try makeRuntime(enforced: true)
        await rt.submitUtterance(sharedRequest)
        XCTAssertEqual(rt.chatTurn?.kind, .ready)
        XCTAssertEqual(rt.chatTurn?.requiresPro, true)

        await rt.buildFromChat()
        let card = try XCTUnwrap(rt.paidConfirm, "有料機能を含む ready の「これでいく」でカードが出る")
        XCTAssertEqual(card.features.map(\.id), ["sharedData"])
        XCTAssertTrue(spy.sent.isEmpty, "カードの間は生成しない")
        XCTAssertTrue(rt.isChatting)

        await rt.buildWithoutPaidFeatures()
        XCTAssertEqual(spy.sent, [card.freeText])
        XCTAssertTrue(spy.sent[0].hasPrefix(sharedRequest) && spy.sent[0].contains("無料版"), "有料機能を抜いた指示で作る: \(spy.sent)")
        XCTAssertNil(rt.paidConfirm)
        XCTAssertFalse(rt.dsl.isVoiceOnly, "アプリができる")
    }

    func testFreeReadyBuildsWithoutCard() async throws {
        let (rt, spy) = try makeRuntime(enforced: true)
        await rt.submitUtterance("毎日の支出を記録する家計簿アプリを作って")
        XCTAssertNotEqual(rt.chatTurn?.requiresPro, true)
        await rt.buildFromChat()
        XCTAssertNil(rt.paidConfirm)
        XCTAssertEqual(spy.sent.count, 1)
    }

    /// Debug の既定(回数制限の無効化中)ではカードを出さずにそのまま作る
    func testNotEnforcedBuildsDirectly() async throws {
        let (rt, spy) = try makeRuntime(enforced: false)
        await rt.submitUtterance(sharedRequest)
        await rt.buildFromChat()
        XCTAssertNil(rt.paidConfirm)
        XCTAssertEqual(spy.sent, [sharedRequest])
    }

    func testDismissReturnsToChatAndTalkingAgainClearsCard() async throws {
        let (rt, spy) = try makeRuntime(enforced: true)
        await rt.submitUtterance(sharedRequest)
        await rt.buildFromChat()
        XCTAssertNotNil(rt.paidConfirm)
        rt.dismissPaidConfirm()
        XCTAssertNil(rt.paidConfirm)
        XCTAssertTrue(rt.isChatting, "やめる = チャットに戻る")

        await rt.buildFromChat()
        XCTAssertNotNil(rt.paidConfirm)
        await rt.submitUtterance("やっぱり自分だけで使う日記アプリにして")
        XCTAssertNil(rt.paidConfirm, "話し直したらカードは閉じる")
        XCTAssertTrue(spy.sent.isEmpty)
    }

    /// edit の案(proposal)は案ごとに判定する
    func testProposalOptionCard() async throws {
        let (rt, spy) = try makeRuntime(enforced: true, example: "kakeibo")
        let paid = ChatOption(id: "opt1", title: "家族で", instruction: "家族で共有して書き込めるようにして", requiresPro: true,
                              paidFeatures: [MockAIClient.sharedDataFeature], freeInstruction: "自分だけの記録のまま見やすくして")
        await rt.choose(paid)
        XCTAssertEqual(rt.paidConfirm?.freeText, "自分だけの記録のまま見やすくして")
        XCTAssertTrue(spy.sent.isEmpty)
        await rt.buildWithoutPaidFeatures()
        XCTAssertEqual(spy.sent, ["自分だけの記録のまま見やすくして"])
    }
}
