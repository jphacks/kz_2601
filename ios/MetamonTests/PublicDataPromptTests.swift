import XCTest
import MetamonDSL
import MetamonServices
import MetamonRenderer
@testable import Metamon

/// #253: 生成直後の「個人の情報も公開されます」の確認を AppRuntime + モック AI で確かめる。
/// `meta.publicPersonalData` がある時だけ出る / 「この欄を消す」「自分だけにする」は修正の指示文を generate に送る。
@MainActor
final class PublicDataPromptTests: XCTestCase {
    final class SpyAI: AIClient, @unchecked Sendable {
        let base = MockAIClient(delay: .milliseconds(20))
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

    private func makeRuntime() throws -> (AppRuntime, SpyAI) {
        let spy = SpyAI()
        var services = BuildConfig.make(environment: .mock)
        services.ai = spy
        let rt = AppRuntime(document: try DSLExamples.document(DSLExamples.initial, version: 0), services: services)
        rt.clarifyEnabled = false
        let defaults = UserDefaults(suiteName: "PublicDataPromptTests.\(UUID().uuidString)")!
        rt.proGate = ProGate(store: ProEntitlementStore(), usage: CreationUsageStore(defaults: defaults),
                             quota: CreationQuota(dailyFreeLimit: 100), limitDisabled: true)
        return (rt, spy)
    }

    private let request = "美容室の予約アプリを作って"
    /// 指示文は UI の言語に合わせる(テストのホストは端末の言語)
    private var english: Bool { LanguagePreference.shared.code == "en" }

    func testPromptAppearsWhenServerReportsPublicPersonalData() async throws {
        let (rt, _) = try makeRuntime()
        await rt.generate(request)
        let prompt = try XCTUnwrap(rt.publicDataPrompt)
        XCTAssertEqual(prompt.fieldNames(), "お名前・電話番号")
        XCTAssertEqual(prompt.version, rt.document.version)
        XCTAssertNotNil(rt.summaryBanner, "summary の表示はそのまま")
    }

    func testNoPromptForPrivateApp() async throws {
        let (rt, _) = try makeRuntime()
        await rt.generate("家計簿アプリを作って")
        XCTAssertNil(rt.publicDataPrompt)
    }

    func testKeepPublicDoesNothing() async throws {
        let (rt, spy) = try makeRuntime()
        await rt.generate(request)
        rt.keepPublic()
        XCTAssertNil(rt.publicDataPrompt)
        XCTAssertEqual(spy.sent, [request])
        XCTAssertEqual(rt.dsl.scope(of: "reservations"), .public)
    }

    func testRemoveFieldsSendsEditInstruction() async throws {
        let (rt, spy) = try makeRuntime()
        await rt.generate(request)
        let prompt = try XCTUnwrap(rt.publicDataPrompt)
        await rt.removePublicPersonalFields()
        XCTAssertEqual(spy.sent.last, prompt.removeInstruction(english: english)) // 予約の「お名前」と「電話番号」の欄を消して
        XCTAssertNil(rt.dsl.entity("reservations")?.field("phone"))
        XCTAssertNil(rt.publicDataPrompt, "消した後の生成では個人の情報が無いので出ない")
    }

    func testMakePrivateSendsEditInstruction() async throws {
        let (rt, spy) = try makeRuntime()
        await rt.generate(request)
        let prompt = try XCTUnwrap(rt.publicDataPrompt)
        await rt.makePublicDataPrivate()
        XCTAssertEqual(spy.sent.last, prompt.privateInstruction(english: english)) // 予約を自分だけの記録にして
        XCTAssertEqual(rt.dsl.scope(of: "reservations"), .private)
    }

    /// 出した後にアプリが別の版に変わっていたら、古い確認からは指示を送らない
    func testStalePromptSendsNothing() async throws {
        let (rt, spy) = try makeRuntime()
        await rt.generate(request)
        let stale = try XCTUnwrap(rt.publicDataPrompt)
        await rt.generate("家計簿にして")
        rt.publicDataPrompt = stale
        await rt.removePublicPersonalFields()
        XCTAssertEqual(spy.sent.count, 2)
        XCTAssertNil(rt.publicDataPrompt)
    }

    func testChatMarksPublicConfirmBubble() async throws {
        let (rt, _) = try makeRuntime()
        rt.clarifyEnabled = true
        rt.clarifyAutoBuild = false
        await rt.submitUtterance("美容室の予約アプリ。お客さんの名前と電話番号も残したい")
        XCTAssertEqual(rt.chatTurn?.kind, .question)
        XCTAssertEqual(rt.chatBubbles.last?.confirmsPublic, true)
        XCTAssertEqual(rt.chatBubbles.first?.confirmsPublic, false)
    }
}
