import Foundation
import XCTest
@testable import MetamonRenderer

/// NFC タグ(NDEF)の読む・書く口(#2233)。組み立てと読み取りの純関数と、偽物での許可・書き込みを確かめる。
final class NFCTagSessionTests: XCTestCase {
    private func roundTrip(_ record: NFCTagRecord) -> NFCTagRecord? {
        NFCNDEFCodec.record(from: NFCNDEFCodec.raw(from: record))
    }

    // MARK: 文字のレコード

    func testTextPayloadKeepsLanguageAndUTF8Body() {
        let payload = NFCNDEFCodec.textPayload("こんにちは", language: "ja")
        XCTAssertEqual([UInt8](payload).first, 2)
        XCTAssertEqual(payload.dropFirst(1).prefix(2), Data("ja".utf8))
        XCTAssertEqual(payload.dropFirst(3), Data("こんにちは".utf8))
    }

    func testTextRoundTripsJapaneseAndEmojiAndNewlines() {
        for text in ["こんにちは", "改行\nを含む 日本語", "🎫 入場券", String(repeating: "あ", count: 500), ""] {
            XCTAssertEqual(roundTrip(.text(text, language: "ja")), .text(text, language: "ja"), text)
        }
    }

    func testTextRoundTripsRegionalLanguageCode() {
        XCTAssertEqual(roundTrip(.text("Hello", language: "en-US")), .text("Hello", language: "en-US"))
    }

    func testEmptyOrOverlongLanguageFallsBackToJa() {
        XCTAssertEqual(roundTrip(.text("x", language: "")), .text("x", language: "ja"))
        XCTAssertEqual(roundTrip(.text("x", language: "  ")), .text("x", language: "ja"))
        XCTAssertEqual(roundTrip(.text("x", language: String(repeating: "a", count: 64))), .text("x", language: "ja"))
    }

    /// 他のアプリが書いた UTF-16 の文字レコードも読める(自分では UTF-8 でしか書かない)
    func testDecodesUTF16TextWrittenByOtherApps() {
        let body = "テスト".data(using: .utf16BigEndian)!
        var payload = Data([UInt8(0x80) | 2])
        payload.append(Data("ja".utf8))
        payload.append(body)
        let decoded = NFCNDEFCodec.decodeText(payload: payload)
        XCTAssertEqual(decoded?.text, "テスト")
        XCTAssertEqual(decoded?.language, "ja")

        var withBOM = Data([UInt8(0x80) | 2])
        withBOM.append(Data("ja".utf8))
        withBOM.append(Data([0xFF, 0xFE]))
        withBOM.append("テスト".data(using: .utf16LittleEndian)!)
        XCTAssertEqual(NFCNDEFCodec.decodeText(payload: withBOM)?.text, "テスト")
    }

    func testBrokenTextPayloadsDecodeToNil() {
        XCTAssertNil(NFCNDEFCodec.decodeText(payload: Data()))
        XCTAssertNil(NFCNDEFCodec.decodeText(payload: Data([5, 0x6A])))
    }

    // MARK: URL のレコード

    func testURIPayloadUsesLongestKnownPrefix() {
        XCTAssertEqual([UInt8](NFCNDEFCodec.uriPayload("https://www.example.com")).first, 2)
        XCTAssertEqual([UInt8](NFCNDEFCodec.uriPayload("https://example.com")).first, 4)
        XCTAssertEqual([UInt8](NFCNDEFCodec.uriPayload("http://www.example.com")).first, 1)
        XCTAssertEqual([UInt8](NFCNDEFCodec.uriPayload("mailto:a@example.com")).first, 6)
        XCTAssertEqual([UInt8](NFCNDEFCodec.uriPayload("tailorit://import/abc")).first, 0)
    }

    func testURIRoundTripsLongAndNonASCIIURLs() {
        let long = "https://tailor-it.app/s/" + String(repeating: "a", count: 900)
        for uri in [long, "https://例え.jp/道/案内?q=日本語#節", "tel:+81312345678", "tailorit://import/abcdef", "urn:nfc:ext:example.com:tag"] {
            XCTAssertEqual(roundTrip(.uri(uri)), .uri(uri), uri)
        }
    }

    func testUnknownPrefixCodeAndEmptyURIDecodeToNil() {
        XCTAssertNil(NFCNDEFCodec.decodeURI(payload: Data([0xFF]) + Data("example.com".utf8)))
        XCTAssertNil(NFCNDEFCodec.decodeURI(payload: Data([0])))
        XCTAssertNil(NFCNDEFCodec.decodeURI(payload: Data()))
    }

    func testPrefixTableMatchesURIRecordTypeDefinition() {
        XCTAssertEqual(NFCNDEFCodec.uriPrefixes.count, 36)
        XCTAssertEqual(NFCNDEFCodec.uriPrefixes[0], "")
        XCTAssertEqual(NFCNDEFCodec.uriPrefixes[35], "urn:nfc:")
    }

    // MARK: 分かる種類だけ拾う

    func testUnknownRecordsAreDropped() {
        let mime = NFCRawRecord(isWellKnown: false, type: Data("text/vcard".utf8), payload: Data("BEGIN:VCARD".utf8))
        let smartPoster = NFCRawRecord(isWellKnown: true, type: Data("Sp".utf8), payload: Data([0x01]))
        let text = NFCNDEFCodec.raw(from: .text("残す", language: "ja"))
        XCTAssertEqual(NFCNDEFCodec.records(from: [mime, smartPoster, text]), [.text("残す", language: "ja")])
        XCTAssertNil(NFCNDEFCodec.record(from: NFCRawRecord(isWellKnown: false, type: Data("T".utf8), payload: Data([0]))))
    }

    func testRecordsKeepTheOrderOnTheTag() {
        let records: [NFCTagRecord] = [.uri("https://example.com"), .text("二番目", language: "ja"), .uri("tel:0312345678")]
        XCTAssertEqual(NFCNDEFCodec.records(from: records.map(NFCNDEFCodec.raw(from:))), records)
    }

    // MARK: 中身を出さない

    func testDescriptionHidesTagContents() {
        XCTAssertEqual(NFCTagRecord.text("秘密", language: "ja").description, "NFCTagRecord(text)")
        XCTAssertEqual(NFCTagRecord.uri("https://secret.example.com").debugDescription, "NFCTagRecord(uri)")
    }

    // MARK: 使えない端末・偽物

    func testUnavailableDeviceThrowsOnReadAndWrite() async {
        let fake = FakeNFCTagSession(isAvailable: false, tag: [.text("x", language: "ja")])
        await assertThrows(NFCTagError.unavailable) { _ = try await fake.read() }
        await assertThrows(NFCTagError.unavailable) { try await fake.write(.uri("https://example.com")) }
    }

    func testEmptyTagThrows() async {
        await assertThrows(NFCTagError.empty) { _ = try await FakeNFCTagSession(tag: []).read() }
    }

    func testWriteStoresRecordAndIsReadBack() async throws {
        let fake = FakeNFCTagSession(tag: [.text("前の中身", language: "ja")])
        try await fake.write(.uri("https://tailor-it.app"))
        XCTAssertEqual(fake.written, [.uri("https://tailor-it.app")])
        let read = try await fake.read()
        XCTAssertEqual(read, [.uri("https://tailor-it.app")])
    }

    func testReadOnlyAndOversizedTagsThrow() async {
        await assertThrows(NFCTagError.readOnly) {
            try await FakeNFCTagSession(writable: false).write(.text("x", language: "ja"))
        }
        await assertThrows(NFCTagError.tooLarge) {
            try await FakeNFCTagSession(capacity: 8).write(.text(String(repeating: "あ", count: 100), language: "ja"))
        }
    }

    func testSampleTagIsReadable() async throws {
        let read = try await FakeNFCTagSession.sample.read()
        XCTAssertEqual(read, [.uri("https://tailor-it.app"), .text("ようこそ", language: "ja")])
    }

    #if canImport(CoreNFC)
    func testLiveSessionNeedsUsageDescription() {
        XCTAssertFalse(LiveNFCTagSession.hasUsageDescription(in: nil))
        XCTAssertFalse(LiveNFCTagSession.hasUsageDescription(in: ["NFCReaderUsageDescription": ""]))
        XCTAssertTrue(LiveNFCTagSession.hasUsageDescription(in: ["NFCReaderUsageDescription": "理由"]))
    }
    #endif

    private func assertThrows(_ expected: NFCTagError, _ body: () async throws -> Void,
                              file: StaticString = #filePath, line: UInt = #line) async {
        do {
            try await body()
            XCTFail("エラーにならなかった", file: file, line: line)
        } catch {
            XCTAssertEqual(error as? NFCTagError, expected, file: file, line: line)
        }
    }
}
