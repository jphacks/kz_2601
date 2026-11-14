import XCTest
import MetamonDSL
@testable import MetamonServices

/// #636: 生成の SSE の読み方(SSEParser の feed・GenerateStreamEvent の decode・status(forCode:))。
final class GenerateStreamParserTests: XCTestCase {
    private func feedAll(_ pieces: [[UInt8]]) -> [SSEFrame] {
        var p = SSEParser()
        return pieces.flatMap { p.feed($0) }
    }

    // MARK: - feed

    /// 多バイト文字の途中で切れた断片も、行が揃ってから文字にする。
    func testFeedJoinsBytesSplitInsideMultibyteCharacter() {
        let bytes = Array("event: app\ndata: {\"name\":\"家計簿\"}\n\n".utf8)
        let cut = bytes.firstIndex(of: 0xE5)! + 1 // 「家」の 1 バイト目の直後
        let frames = feedAll([Array(bytes[..<cut]), Array(bytes[cut...])])
        XCTAssertEqual(frames, [SSEFrame(event: "app", data: #"{"name":"家計簿"}"#)])
    }

    /// 1 バイトずつ流しても同じ 1 件になる(`AsyncBytes` の道)。
    func testFeedByteGivesSameFrames() {
        var p = SSEParser()
        var frames: [SSEFrame] = []
        for b in "event: restart\r\ndata: {\"attempt\":2}\r\n\r\ndata: x\n\n".utf8 {
            if let f = p.feed(byte: b) { frames.append(f) }
        }
        XCTAssertEqual(frames, [SSEFrame(event: "restart", data: #"{"attempt":2}"#), SSEFrame(event: "message", data: "x")])
    }

    /// 空行が続いても空の 1 件は出さない。event だけで data の無い塊も出さず、次の塊に event を持ち越さない。
    func testBlankLinesAndEventWithoutDataYieldNothing() {
        let frames = feedAll([Array("\n\n\nevent: app\n\ndata: y\n\n".utf8)])
        XCTAssertEqual(frames, [SSEFrame(event: "message", data: "y")])
    }

    /// 値の前の空白は 1 つだけ取る。`:` の無い行は値が空の field。
    func testFieldValueSpacing() {
        let frames = feedAll([Array("data:  two\ndata\nevent:result\n\n".utf8)])
        XCTAssertEqual(frames, [SSEFrame(event: "result", data: " two\n")])
    }

    // MARK: - decode

    private func decode(_ event: String, _ data: String) -> GenerateStreamEvent? {
        GenerateStreamEvent.decode(SSEFrame(event: event, data: data))
    }

    func testUnknownEventsAreSkipped() {
        for event in ["message", "summary", "ping", "APP", ""] {
            XCTAssertNil(decode(event, #"{"name":"x"}"#), event)
        }
    }

    func testResultKeepsBodyAsIs() {
        let body = #"{"appId":"a1","version":3}"#
        guard case .result(let data)? = decode("result", body) else { return XCTFail("result") }
        XCTAssertEqual(String(decoding: data, as: UTF8.self), body)
    }

    func testRestartWithoutAttemptIsZero() {
        guard case .progress(.restart(0))? = decode("restart", "{}") else { return XCTFail("restart") }
        guard case .progress(.restart(0))? = decode("restart", "壊れた") else { return XCTFail("壊れた restart も 0") }
    }

    func testBrokenErrorBodyBecomesHTTP500() {
        guard case .failure(let e)? = decode("error", "壊れた") else { return XCTFail("error") }
        guard case .http(let status, let body) = e else { return XCTFail("\(e)") }
        XCTAssertEqual(status, 500)
        XCTAssertEqual(body, "壊れた")
        guard case .failure(.http(500, _))? = decode("error", #"{"error":{"message":"code が無い"}}"#) else {
            return XCTFail("code の無い error も 500")
        }
    }

    /// status の無い error は code から status を決める。status があればそれを使う。
    func testErrorWithoutStatusUsesCode() {
        func status(_ code: String) -> Int? {
            guard case .failure(let e)? = decode("error", #"{"error":{"code":"\#(code)","message":"m"}}"#) else { return nil }
            return e.status
        }
        XCTAssertEqual(status(ProRequired.code), 402)
        XCTAssertEqual(status(PlanLimit.code), 403)
        XCTAssertEqual(status("upstream_timeout"), 500)
        guard case .failure(let e)? = decode("error", #"{"error":{"code":"\#(PlanLimit.code)","message":"m","status":429}}"#) else {
            return XCTFail("error")
        }
        XCTAssertEqual(e.status, 429, "本文の status が先")
    }

    // MARK: - status(forCode:)

    func testStatusForCode() {
        XCTAssertEqual(GenerateStreamEvent.status(forCode: AppLimit.code), 402)
        XCTAssertEqual(GenerateStreamEvent.status(forCode: ProRequired.code), 402)
        XCTAssertEqual(GenerateStreamEvent.status(forCode: "coins_insufficient"), 402)
        XCTAssertEqual(GenerateStreamEvent.status(forCode: "allowance_exhausted"), 402)
        XCTAssertEqual(GenerateStreamEvent.status(forCode: "overflow_cap_reached"), 402)
        XCTAssertEqual(GenerateStreamEvent.status(forCode: PlanLimit.code), 403)
        for code in ["", "upstream_timeout", "rate_limited", "APP_LIMIT_REACHED"] {
            XCTAssertEqual(GenerateStreamEvent.status(forCode: code), 500, code)
        }
    }
}
