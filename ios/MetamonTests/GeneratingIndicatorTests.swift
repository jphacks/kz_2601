import XCTest
import MetamonDSL
import MetamonServices
@testable import Metamon

/// #182: 生成を始めた瞬間から完成まで `morphJob`(RootView の生成中表示)が立ち続け、完成・失敗で下りる。
@MainActor
final class GeneratingIndicatorTests: XCTestCase {
    private func runtime(example: String, ai: any AIClient) throws -> AppRuntime {
        var services = BuildConfig.make(environment: .mock)
        services.ai = ai
        let version = example == DSLExamples.initial ? 0 : 1
        return AppRuntime(document: try DSLExamples.document(example, version: version), services: services)
    }

    /// `work` の間、生成中の種類を 5ms 刻みで見て変化だけを記録する(nil → kind → nil なら途切れていない)。
    private func observeKinds(_ rt: AppRuntime, while work: @escaping @MainActor () async -> Void) async -> [AppRuntime.MorphJob.Kind?] {
        var seen: [AppRuntime.MorphJob.Kind?] = [rt.morphJob?.kind]
        var done = false
        let task = Task { @MainActor in await work(); done = true }
        while !done {
            try? await Task.sleep(for: .milliseconds(5))
            let kind = rt.morphJob?.kind
            if seen.last != kind { seen.append(kind) }
        }
        await task.value
        let final = rt.morphJob?.kind
        if seen.last.flatMap({ $0 }) != final { seen.append(final) }
        return seen
    }

    func testKindDecision() {
        XCTAssertEqual(AppRuntime.MorphJob.kind(isVoiceOnly: true, transcript: "合計を大きく"), .create)
        XCTAssertEqual(AppRuntime.MorphJob.kind(isVoiceOnly: false, transcript: "筋トレの記録アプリを作って"), .create)
        XCTAssertEqual(AppRuntime.MorphJob.kind(isVoiceOnly: false, transcript: "合計をもっと大きく"), .edit)
    }

    /// 対話なし(設定 OFF)の即生成
    func testCreateFromInitialShowsFullScreenUntilDone() async throws {
        let rt = try runtime(example: DSLExamples.initial, ai: MockAIClient(delay: .milliseconds(300)))
        rt.clarifyEnabled = false
        let kinds = await observeKinds(rt) { await rt.submitUtterance("家計簿アプリを作って") }
        XCTAssertEqual(kinds, [nil, .create, nil])
        XCTAssertFalse(rt.dsl.isVoiceOnly)
    }

    /// 対話あり →「この内容で作る」
    func testBuildFromChatShowsFullScreenUntilDone() async throws {
        let rt = try runtime(example: DSLExamples.initial, ai: MockAIClient(delay: .milliseconds(300)))
        rt.clarifyEnabled = true
        await rt.submitUtterance("毎日の支出を記録する家計簿アプリを作って") // モックは長い 1 発目で ready
        XCTAssertEqual(rt.chatTurn?.kind, .ready)
        let kinds = await observeKinds(rt) { await rt.buildFromChat() }
        XCTAssertEqual(kinds, [nil, .create, nil], "「この内容で作る」から完成まで全画面の生成中が途切れない")
        XCTAssertFalse(rt.isChatting)
        XCTAssertFalse(rt.dsl.isVoiceOnly)
    }

    /// 生成後のアプリから「〇〇アプリを作って」(作り直し)も全画面
    func testWholeAppRequestFromGeneratedAppIsFullScreen() async throws {
        let rt = try runtime(example: "kakeibo", ai: MockAIClient(delay: .milliseconds(300)))
        let kinds = await observeKinds(rt) { await rt.morph(transcript: "筋トレの記録アプリを作って") }
        XCTAssertEqual(kinds, [nil, .create, nil])
    }

    func testEditStaysInline() async throws {
        let rt = try runtime(example: "kakeibo", ai: MockAIClient(delay: .milliseconds(300)))
        let kinds = await observeKinds(rt) { await rt.morph(transcript: "合計をもっと大きく") }
        XCTAssertEqual(kinds, [nil, .edit, nil])
    }

    func testFailureSwitchesToErrorCard() async throws {
        let rt = try runtime(example: DSLExamples.initial, ai: FailingAI())
        rt.clarifyEnabled = false
        let kinds = await observeKinds(rt) { await rt.submitUtterance("家計簿アプリを作って") }
        XCTAssertEqual(kinds, [nil, .create, nil])
        XCTAssertNotNil(rt.errorCard, "失敗したら状態カードに切り替わる")
        XCTAssertTrue(rt.dsl.isVoiceOnly)
    }
}

private struct FailingAI: AIClient {
    func morph(transcript: String, appId: String, currentVersion: Int) async throws -> MorphResult {
        try await Task.sleep(for: .milliseconds(200))
        throw URLError(.timedOut)
    }
    func chat(appId: String, messages: [ChatMessage], locale: String?, mode: ChatMode?) async throws -> ChatTurn { throw URLError(.timedOut) }
    func resetChat(appId: String) async {}
}
