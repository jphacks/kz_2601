import XCTest
import MetamonDSL
import MetamonServices
@testable import Metamon

/// 「1 つ前に戻す」/「やり直す」の入口(#209)。モック(端末内の履歴で戻す経路)で状態遷移を確かめる。
@MainActor
final class UndoRedoTests: XCTestCase {
    /// 生成を 1 回だけ止めておけるモック。止めた生成は `started` で始まりを知らせ、`release()` まで返らない(実時間の sleep に頼らない)。
    final class GatedAI: AIClient, @unchecked Sendable {
        let base = MockAIClient(delay: .zero)
        private let lock = NSLock()
        private var holdNext = false
        private var release: CheckedContinuation<Void, Never>?
        private var released = false
        let started: AsyncStream<Void>
        private let startedSink: AsyncStream<Void>.Continuation

        init() { (started, startedSink) = AsyncStream.makeStream() }

        func holdNextMorph() { lock.withLock { holdNext = true } }

        func releaseMorph() {
            let waiting = lock.withLock { () -> CheckedContinuation<Void, Never>? in
                released = true
                defer { release = nil }
                return release
            }
            waiting?.resume()
        }

        func morph(transcript: String, appId: String, currentVersion: Int) async throws -> MorphResult {
            let hold = lock.withLock { () -> Bool in defer { holdNext = false }; return holdNext }
            if hold {
                startedSink.yield()
                await withCheckedContinuation { cont in
                    let resumeNow = lock.withLock { () -> Bool in
                        if released { return true }
                        release = cont
                        return false
                    }
                    if resumeNow { cont.resume() }
                }
            }
            return try await base.morph(transcript: transcript, appId: appId, currentVersion: currentVersion)
        }
        func chat(appId: String, messages: [ChatMessage], locale: String?, mode: ChatMode?) async throws -> ChatTurn {
            try await base.chat(appId: appId, messages: messages, locale: locale, mode: mode)
        }
        func resetChat(appId: String) async {}
    }

    /// 既定のモック(1.2 秒の実時間の待ち)は使わない。全体実行で 1 件が長く走るほど、テストのプロセスが落ちた時に巻き込まれやすい(#568)
    private func makeRuntime(ai: AIClient = MockAIClient(delay: .milliseconds(10))) throws -> AppRuntime {
        var services = BuildConfig.make(environment: .mock)
        services.ai = ai
        let doc = try DSLExamples.document("kakeibo", version: 1)
        let rt = AppRuntime(document: doc, services: services)
        rt.clarifyEnabled = false
        return rt
    }

    func testUndoHiddenUntilThereIsAPreviousVersion() async throws {
        let rt = try makeRuntime()
        XCTAssertFalse(rt.canUndo, "version 1(履歴なし)では戻せない")
        XCTAssertNil(rt.redoPoint)

        await rt.generate("金額を大きくして")
        XCTAssertEqual(rt.document.version, 2)
        XCTAssertTrue(rt.canUndo, "修正の後は戻せる")
        XCTAssertEqual(rt.summaryBanner?.action, .undo, "修正直後の要約に「元に戻す」")
    }

    func testInitialScreenCannotUndo() throws {
        let services = BuildConfig.make(environment: .mock)
        let doc = try DSLExamples.document(DSLExamples.initial, version: 0)
        let rt = AppRuntime(document: doc, services: services)
        XCTAssertFalse(rt.canUndo, "初期(音声)画面には戻す対象が無い")
    }

    func testUndoThenRedo() async throws {
        let rt = try makeRuntime()
        let original = rt.dsl.app.name
        await rt.generate("筋トレの記録")
        let edited = rt.dsl.app.name
        XCTAssertNotEqual(original, edited, "モックの修正で画面が変わる前提")

        await rt.undo(expectedVersion: rt.document.version)
        XCTAssertEqual(rt.document.version, 1)
        XCTAssertEqual(rt.dsl.app.name, original, "1 つ前の画面に戻る")
        XCTAssertEqual(rt.summaryBanner?.text, L10n.tr("元に戻しました"))
        XCTAssertEqual(rt.summaryBanner?.action, .redo, "戻した直後は「やり直す」")
        XCTAssertNotNil(rt.redoPoint)
        XCTAssertFalse(rt.canUndo, "これ以上前は無い")

        await rt.redo(expectedVersion: rt.document.version)
        XCTAssertEqual(rt.document.version, 2)
        XCTAssertEqual(rt.dsl.app.name, edited, "戻す前の画面に戻る")
        XCTAssertNil(rt.redoPoint, "やり直したら「やり直す」は消える")
        XCTAssertTrue(rt.canUndo, "やり直した後はまた戻せる")
        XCTAssertEqual(rt.summaryBanner?.action, .undo)
    }

    func testDoubleTapUndoesOnlyOnce() async throws {
        let rt = try makeRuntime()
        await rt.generate("金額を大きくして")
        await rt.generate("筋トレの記録")
        XCTAssertEqual(rt.document.version, 3)

        // 同じフレームで 2 回押された(どちらも押した時点の版は 3)
        let tapped = rt.document.version
        async let first: Void = rt.undo(expectedVersion: tapped)
        async let second: Void = rt.undo(expectedVersion: tapped)
        _ = await (first, second)
        XCTAssertEqual(rt.document.version, 2, "連打でも 1 つだけ戻る")
        XCTAssertTrue(rt.canUndo)

        let again = rt.document.version
        async let r1: Void = rt.redo(expectedVersion: again)
        async let r2: Void = rt.redo(expectedVersion: again)
        _ = await (r1, r2)
        XCTAssertEqual(rt.document.version, 3, "やり直すの連打も 1 回だけ")
    }

    func testUndoIgnoredWhileMorphing() async throws {
        let ai = GatedAI()
        let rt = try makeRuntime(ai: ai)
        await rt.generate("金額を大きくして")
        ai.holdNextMorph()
        let job = Task { await rt.generate("筋トレの記録") }
        // 生成が AI に届くまで待つ(100ms の sleep だと、混んでいる時に生成が始まる前・終わった後に戻してしまう・#568)
        var started = ai.started.makeAsyncIterator()
        _ = await started.next()
        XCTAssertTrue(rt.isMorphing)
        await rt.undo()
        XCTAssertEqual(rt.document.version, 2, "生成中は戻さない")
        ai.releaseMorph()
        await job.value
        XCTAssertEqual(rt.document.version, 3)
    }

    func testNewEditClearsRedo() async throws {
        let rt = try makeRuntime()
        await rt.generate("金額を大きくして")
        await rt.undo()
        XCTAssertNotNil(rt.redoPoint)
        await rt.generate("筋トレの記録")
        XCTAssertNil(rt.redoPoint, "戻した後に別の修正をしたら「やり直す」は消える")
    }
}
