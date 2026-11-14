import XCTest
@testable import MetamonRenderer

/// 端末の中の翻訳の口(#1999): 偽物の状態ごとの返り方・上限・iOS 17(使えない)。
final class OnDeviceTranslatorTests: XCTestCase {
    private let ja = Locale.Language(identifier: "ja")
    private let en = Locale.Language(identifier: "en")
    private let xx = Locale.Language(identifier: "tlh")

    // MARK: 状態ごとの返り方

    func testInstalledTranslatesEachText() async throws {
        let t = FakeOnDeviceTranslator()
        let status = await t.availability(from: ja, to: en)
        XCTAssertEqual(status, .installed)
        let out = try await t.translate(["こんにちは", "ありがとう"], from: ja, to: en)
        XCTAssertEqual(out, ["[en] こんにちは", "[en] ありがとう"])
    }

    func testSupportedNeedsPrepareThenTranslates() async throws {
        let t = FakeOnDeviceTranslator(statuses: [.init("ja", "en"): .supported])
        do {
            _ = try await t.translate(["a"], from: ja, to: en)
            XCTFail("未ダウンロードで訳せてしまった")
        } catch {
            XCTAssertEqual(error as? OnDeviceTranslationError, .notInstalled)
        }
        try await t.prepare(from: ja, to: en)
        let status = await t.availability(from: ja, to: en)
        XCTAssertEqual(status, .installed)
        let out = try await t.translate(["a"], from: ja, to: en)
        XCTAssertEqual(out, ["[en] a"])
    }

    func testPrepareCancelledStaysSupported() async {
        let t = FakeOnDeviceTranslator(statuses: [.init("ja", "en"): .supported], downloadSucceeds: false)
        do {
            try await t.prepare(from: ja, to: en)
            XCTFail("閉じたのに成功した")
        } catch {
            XCTAssertEqual(error as? OnDeviceTranslationError, .failed)
        }
        let status = await t.availability(from: ja, to: en)
        XCTAssertEqual(status, .supported)
    }

    func testUnsupportedPairThrows() async {
        let t = FakeOnDeviceTranslator(statuses: [.init("ja", "tlh"): .unsupported])
        do {
            _ = try await t.translate(["a"], from: ja, to: xx)
            XCTFail()
        } catch {
            XCTAssertEqual(error as? OnDeviceTranslationError, .unsupported)
        }
        do {
            try await t.prepare(from: ja, to: xx)
            XCTFail()
        } catch {
            XCTAssertEqual(error as? OnDeviceTranslationError, .unsupported)
        }
    }

    func testGuidance() {
        XCTAssertNil(OnDeviceTranslationStatus.installed.guidance)
        XCTAssertNotNil(OnDeviceTranslationStatus.supported.guidance)
        XCTAssertNotNil(OnDeviceTranslationStatus.unsupported.guidance)
    }

    // MARK: 上限

    func testClipLimitsCountLengthAndTotal() {
        let many = Array(repeating: "a", count: OnDeviceTranslationLimit.maxTexts + 5)
        XCTAssertEqual(OnDeviceTranslationLimit.clip(many).count, OnDeviceTranslationLimit.maxTexts)

        let long = String(repeating: "あ", count: OnDeviceTranslationLimit.maxCharactersPerText + 10)
        XCTAssertEqual(OnDeviceTranslationLimit.clip([long]).first?.count, OnDeviceTranslationLimit.maxCharactersPerText)

        let texts = Array(repeating: long, count: 10)
        let clipped = OnDeviceTranslationLimit.clip(texts)
        XCTAssertEqual(clipped.reduce(0) { $0 + $1.count }, OnDeviceTranslationLimit.maxTotalCharacters)
        XCTAssertEqual(clipped.count, OnDeviceTranslationLimit.maxTotalCharacters / OnDeviceTranslationLimit.maxCharactersPerText)
    }

    func testTranslateAppliesClip() async throws {
        let t = FakeOnDeviceTranslator()
        let long = String(repeating: "x", count: OnDeviceTranslationLimit.maxCharactersPerText * 2)
        let out = try await t.translate([long], from: ja, to: en)
        XCTAssertEqual(out.first?.count, "[en] ".count + OnDeviceTranslationLimit.maxCharactersPerText)
        let batches = await t.translatedBatches
        XCTAssertEqual(batches.first?.first?.count, OnDeviceTranslationLimit.maxCharactersPerText)
    }

    // MARK: iOS 17・mock の選び方

    @MainActor func testMockUsesFake() {
        XCTAssertTrue(OnDeviceTranslators.make(mock: true) is FakeOnDeviceTranslator)
    }

    /// 本物を載せる見えない View が無い(=iOS 17 / macOS と同じ扱い)と使えない。
    @MainActor func testWithoutHostIsUnsupported() async {
        let t = OnDeviceTranslators.make(mock: false, host: nil)
        let status = await t.availability(from: ja, to: en)
        XCTAssertEqual(status, .unsupported)
    }

    func testUnsupportedTranslatorReportsUnsupported() async {
        let t = UnsupportedOnDeviceTranslator()
        let status = await t.availability(from: ja, to: en)
        XCTAssertEqual(status, .unsupported)
        do {
            _ = try await t.translate(["a"], from: ja, to: en)
            XCTFail()
        } catch {
            XCTAssertEqual(error as? OnDeviceTranslationError, .unsupported)
        }
    }

    // MARK: 依頼の順番待ち(本物の host が使う)

    /// 投げた Task が submit まで進むのを待つ
    @MainActor private func waitCount(_ q: OnDeviceTranslationJobQueue<String>, _ n: Int) async {
        for _ in 0..<1000 where q.count < n { await Task.yield() }
        XCTAssertEqual(q.count, n)
    }

    @MainActor func testQueueIgnoresActionForOtherPairAndRedispatches() async throws {
        let q = OnDeviceTranslationJobQueue<String>()
        var dispatched: [String] = []
        q.dispatch = { s, t in dispatched.append("\(s.minimalIdentifier)>\(t.minimalIdentifier)") }
        let task = Task { try await q.submit("a", from: ja, to: en) }
        await waitCount(q, 1)
        XCTAssertEqual(dispatched, ["ja>en"])
        XCTAssertNil(q.begin(source: en, target: ja), "前の組の session で処理してしまった")
        XCTAssertEqual(dispatched, ["ja>en", "ja>en"])
        let job = try XCTUnwrap(q.begin(source: ja, target: en))
        XCTAssertNil(q.begin(source: ja, target: en), "同じ依頼を二重に始めた")
        q.finish(job.token, .success(["A"]))
        let out = try await task.value
        XCTAssertEqual(out, ["A"])
        XCTAssertEqual(q.count, 0)
    }

    @MainActor func testQueueStaleActionDoesNotDropRunningJob() async throws {
        let q = OnDeviceTranslationJobQueue<String>()
        let first = Task { try await q.submit("1", from: ja, to: en) }
        await waitCount(q, 1)
        let job1 = try XCTUnwrap(q.begin(source: ja, target: en))
        let second = Task { try await q.submit("2", from: ja, to: en) }
        await waitCount(q, 2)
        // 処理中に action がもう一度来ても、流し中の印は消えず 2 件目は先に始まらない
        XCTAssertNil(q.begin(source: ja, target: en))
        XCTAssertEqual(q.count, 2)
        q.finish(job1.token, .success(["one"]))
        let job2 = try XCTUnwrap(q.begin(source: ja, target: en))
        XCTAssertEqual(job2.payload, "2")
        q.finish(job1.token, .success(["late"])) // 古い token は捨てる
        q.finish(job2.token, .success(["two"]))
        let a = try await first.value
        let b = try await second.value
        XCTAssertEqual(a, ["one"])
        XCTAssertEqual(b, ["two"])
    }

    @MainActor func testQueueTimesOutWhenActionNeverFires() async throws {
        let q = OnDeviceTranslationJobQueue<String>(fireTimeout: .milliseconds(50))
        let first = Task { try await q.submit("1", from: ja, to: en) }
        let second = Task { try await q.submit("2", from: ja, to: en) }
        do {
            _ = try await first.value
            XCTFail("action が来ないのに返ってきた")
        } catch {
            XCTAssertEqual(error as? OnDeviceTranslationError, .failed)
        }
        // 次の依頼は流れる
        let job2 = try XCTUnwrap(q.begin(source: ja, target: en))
        q.finish(job2.token, .success(["two"]))
        let b = try await second.value
        XCTAssertEqual(b, ["two"])
    }

    @MainActor func testQueueStartedJobDoesNotTimeOut() async throws {
        let q = OnDeviceTranslationJobQueue<String>(fireTimeout: .milliseconds(30))
        let task = Task { try await q.submit("1", from: ja, to: en) }
        await waitCount(q, 1)
        let job = try XCTUnwrap(q.begin(source: ja, target: en))
        try await Task.sleep(for: .milliseconds(100)) // ダウンロードの画面などで長くかかる
        q.finish(job.token, .success(["ok"]))
        let out = try await task.value
        XCTAssertEqual(out, ["ok"])
    }

    @MainActor func testQueueFailAllReturnsEveryJob() async {
        let q = OnDeviceTranslationJobQueue<String>()
        let a = Task { try await q.submit("1", from: ja, to: en) }
        let b = Task { try await q.submit("2", from: ja, to: en) }
        await waitCount(q, 2)
        q.failAll(OnDeviceTranslationError.hostMissing)
        for t in [a, b] {
            do {
                _ = try await t.value
                XCTFail()
            } catch {
                XCTAssertEqual(error as? OnDeviceTranslationError, .hostMissing)
            }
        }
        XCTAssertEqual(q.count, 0)
    }
}
