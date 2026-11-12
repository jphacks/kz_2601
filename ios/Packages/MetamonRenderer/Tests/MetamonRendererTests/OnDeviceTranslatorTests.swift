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
}
