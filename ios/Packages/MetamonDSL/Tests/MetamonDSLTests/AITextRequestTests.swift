import XCTest
@testable import MetamonDSL

/// AI の文の依頼の整え方(件数の端・long / transform の判定・材料の上限・clip)。
final class AITextRequestTests: XCTestCase {
    private func request(count: Int? = nil, task: String? = nil, length: String? = nil) -> AITextRequest {
        AITextRequest(instruction: "x", shape: .list, count: count, task: task, length: length)
    }

    func testResolvedCountDefaultsAndClampsToBothEnds() {
        XCTAssertEqual(request().resolvedCount, AITextLimits.defaultCount)
        XCTAssertEqual(request(count: 0).resolvedCount, 1, "下の端は 1")
        XCTAssertEqual(request(count: -5).resolvedCount, 1)
        XCTAssertEqual(request(count: 1).resolvedCount, 1)
        XCTAssertEqual(request(count: AITextLimits.maxItems).resolvedCount, AITextLimits.maxItems)
        XCTAssertEqual(request(count: AITextLimits.maxItems + 1).resolvedCount, AITextLimits.maxItems)
    }

    func testIsLongOnlyForLong() {
        XCTAssertTrue(request(length: "long").isLong)
        XCTAssertFalse(request().isLong)
        XCTAssertFalse(request(length: "LONG").isLong, "long 以外の綴りは short")
        XCTAssertNil(request(length: "medium").length)
    }

    func testIsTransformExceptGenerateAndNil() {
        XCTAssertFalse(request().isTransform)
        XCTAssertFalse(request(task: "generate").isTransform)
        XCTAssertTrue(request(task: "summarize").isTransform)
        XCTAssertTrue(request(task: "chat").isTransform)
    }

    func testMaxInputBytesFollowsLength() {
        XCTAssertEqual(AITextLimits.maxInputBytes(long: false), AITextLimits.maxInputBytes)
        XCTAssertEqual(AITextLimits.maxInputBytes(long: true), AITextLimits.maxLongInputBytes)
        XCTAssertGreaterThan(AITextLimits.maxLongInputBytes, AITextLimits.maxInputBytes)
    }

    func testClipCutsByBytesWithoutBreakingCharacters() {
        XCTAssertEqual(AITextLimits.clip("abcdef", maxBytes: 6), "abcdef", "ちょうどは切らない")
        XCTAssertEqual(AITextLimits.clip("abcdef", maxBytes: 4), "abcd")
        XCTAssertEqual(AITextLimits.clip("あいう", maxBytes: 7), "あい", "3 バイトの文字の途中では切らない")
        XCTAssertEqual(AITextLimits.clip("a😀b", maxBytes: 4), "a", "4 バイトの絵文字を半分にしない")
        XCTAssertEqual(AITextLimits.clip("a😀b", maxBytes: 5), "a😀")
        let flag = "🇯🇵" // 2 つのスカラーで 1 文字(8 バイト)
        XCTAssertEqual(AITextLimits.clip("x" + flag, maxBytes: 6), "x", "1 文字の途中では切らない")
        XCTAssertEqual(AITextLimits.clip("abc", maxBytes: 0), "")
    }

    func testClipUsesLongLimitWhenGiven() {
        let s = String(repeating: "a", count: AITextLimits.maxLongInputBytes + 10)
        XCTAssertEqual(AITextLimits.clip(s).utf8.count, AITextLimits.maxInputBytes)
        XCTAssertEqual(AITextLimits.clip(s, maxBytes: AITextLimits.maxInputBytes(long: true)).utf8.count,
                       AITextLimits.maxLongInputBytes)
    }
}
