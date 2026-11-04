#if !APP_STORE_REVIEW
import XCTest
import MetamonDSL
@testable import MetamonServices

/// #507: 仲間探しの API クライアント(パス・メソッド・本文)。
final class MatchingClientTests: XCTestCase {
    private var savedSession: URLSession!
    private let base = URL(string: "https://appconfig.test/")!
    private var client: HTTPMatchingClient { HTTPMatchingClient(baseURL: base, token: StaticTokenProvider("u1")) }

    override func setUp() {
        super.setUp()
        savedSession = HTTP.session
        HTTP.session = HTTP.makeSession(protocolClasses: [StubURLProtocol.self])
    }

    override func tearDown() {
        HTTP.session = savedSession
        super.tearDown()
    }

    private var requests: [URLRequest] { StubURLProtocol.lock.withLock { StubURLProtocol.requests } }

    private func body(_ req: URLRequest) -> [String: Any]? {
        var data = req.httpBody
        if data == nil, let stream = req.httpBodyStream {
            stream.open()
            defer { stream.close() }
            var buf = [UInt8](repeating: 0, count: 4096)
            var out = Data()
            while stream.hasBytesAvailable {
                let n = stream.read(&buf, maxLength: buf.count)
                if n <= 0 { break }
                out.append(buf, count: n)
            }
            data = out
        }
        guard let data, !data.isEmpty else { return nil }
        return try? JSONSerialization.jsonObject(with: data) as? [String: Any]
    }

    func testEndpoints() async throws {
        let one = #"{"uid":"u2","profileRecordId":"r2","status":"pending","updatedAt":"2026-11-02T10:00:00Z"}"#
        StubURLProtocol.reset([
            .init(status: 200, body: one),
            .init(status: 200, body: #"{"items":[\#(one)]}"#),
            .init(status: 200, body: #"{"items":[]}"#),
            .init(status: 200, body: #"{"items":null}"#),
            .init(status: 200, body: #"{"uid":"u2","status":"matched","matchedAt":"2026-11-02T10:00:00Z"}"#),
            .init(status: 204),
            .init(status: 204),
        ])
        let sent = try await client.sendInterest(appId: "app1", recordId: "r2", fromRecordId: "r1", ageConfirmed: true)
        XCTAssertEqual(sent.uid, "u2")
        let incoming = try await client.list(appId: "app1", box: .incoming)
        XCTAssertEqual(incoming.map(\.uid), ["u2"])
        let outgoing = try await client.list(appId: "app1", box: .outgoing)
        XCTAssertEqual(outgoing, [])
        let matches = try await client.list(appId: "app1", box: .matches)
        XCTAssertEqual(matches, [], "items が null でも空")
        let accepted = try await client.accept(appId: "app1", uid: "u2", ageConfirmed: true)
        XCTAssertTrue(accepted.isMatched)
        try await client.decline(appId: "app1", uid: "u2")
        try await client.unmatch(appId: "app1", uid: "u2")

        let r = requests
        XCTAssertEqual(r.map { "\($0.httpMethod ?? "") \($0.url?.path ?? "")\($0.url?.query.map { "?" + $0 } ?? "")" }, [
            "POST /v1/apps/app1/interests",
            "GET /v1/apps/app1/interests?box=incoming",
            "GET /v1/apps/app1/interests?box=outgoing",
            "GET /v1/apps/app1/matches",
            "POST /v1/apps/app1/interests/u2/accept",
            "DELETE /v1/apps/app1/interests/u2",
            "DELETE /v1/apps/app1/matches/u2",
        ])
        let send = try XCTUnwrap(body(r[0]))
        XCTAssertEqual(send["recordId"] as? String, "r2")
        XCTAssertEqual(send["fromRecordId"] as? String, "r1")
        XCTAssertEqual(send["ageConfirmed"] as? Bool, true)
        XCTAssertNil(send["age"], "年齢そのものは送らない")
        XCTAssertEqual(body(r[4])?["ageConfirmed"] as? Bool, true)
    }

    func testAgeUnconfirmedSurfacesReason() async throws {
        StubURLProtocol.reset([.init(status: 400, body: #"{"error":{"code":"bad_request","message":"confirm","details":{"reason":"age_unconfirmed"}}}"#)])
        do {
            _ = try await client.sendInterest(appId: "app1", recordId: "r2", fromRecordId: "r1", ageConfirmed: true)
            XCTFail("must throw")
        } catch let APIError.server(status, _, _, details) {
            XCTAssertEqual(status, 400)
            XCTAssertEqual(details["reason"], .string(MatchingReason.ageUnconfirmed))
        }
    }
}

/// 呼ばれた操作を記録し、`error` があれば失敗する。`probe` は応答の前に呼ぶ(楽観的な反映を途中で見る)。
final class FakeMatchingClient: MatchingClient, @unchecked Sendable {
    private let lock = NSLock()
    var error: Error?
    var lists: [MatchBox: [MatchInterest]] = [:]
    var sendResult: MatchInterest?
    var probe: (@Sendable () async -> Void)?
    private(set) var calls: [String] = []

    private func record(_ call: String) async throws {
        lock.withLock { calls.append(call) }
        await probe?()
        if let error { throw error }
    }

    func sendInterest(appId: String, recordId: String, fromRecordId: String, ageConfirmed: Bool) async throws -> MatchInterest {
        try await record("send \(recordId) from \(fromRecordId) age=\(ageConfirmed)")
        return sendResult ?? MatchInterest(uid: "u2", profileRecordId: recordId)
    }
    func list(appId: String, box: MatchBox) async throws -> [MatchInterest] {
        try await record("list \(box.rawValue)")
        return lists[box] ?? []
    }
    func accept(appId: String, uid: String, ageConfirmed: Bool) async throws -> MatchInterest {
        try await record("accept \(uid) age=\(ageConfirmed)")
        return MatchInterest(uid: uid, profileRecordId: "r-\(uid)", status: "matched")
    }
    func decline(appId: String, uid: String) async throws { try await record("decline \(uid)") }
    func unmatch(appId: String, uid: String) async throws { try await record("unmatch \(uid)") }
}

/// #507: 楽観的に反映し、失敗したら操作前に戻す。
@MainActor
final class MatchingStoreTests: XCTestCase {
    private func loaded(_ fake: FakeMatchingClient) async throws -> MatchingStore {
        fake.lists = [
            .incoming: [MatchInterest(uid: "u3", profileRecordId: "r3")],
            .outgoing: [],
            .matches: [MatchInterest(uid: "u4", profileRecordId: "r4", status: "matched")],
        ]
        let store = MatchingStore(appId: "app1", client: fake)
        for box in MatchBox.allCases { try await store.load(box) }
        return store
    }

    func testLoadAndState() async throws {
        let store = try await loaded(FakeMatchingClient())
        XCTAssertEqual(store.state(of: "u3"), .incoming)
        XCTAssertEqual(store.state(of: "u4"), .matched)
        XCTAssertEqual(store.state(of: "u9"), .none)
        XCTAssertEqual(store.items(.outgoing), [])
    }

    func testSendShowsSentBeforeTheServerAnswers() async throws {
        let fake = FakeMatchingClient()
        let store = try await loaded(fake)
        nonisolated(unsafe) var during: (InterestState, Bool)?
        fake.probe = { await MainActor.run { during = (store.state(of: "u2"), store.isBusy("u2")) } }
        try await store.send(to: "u2", recordId: "r2", fromRecordId: "r1")
        XCTAssertEqual(during?.0, .sent, "応答の前に「送りました」")
        XCTAssertEqual(during?.1, true, "送信中は押せない")
        XCTAssertEqual(store.state(of: "u2"), .sent)
        XCTAssertFalse(store.isBusy("u2"))
        XCTAssertEqual(fake.calls.last, "send r2 from r1 age=true")
    }

    func testSendRollsBackOnFailure() async throws {
        let fake = FakeMatchingClient()
        let store = try await loaded(fake)
        fake.error = URLError(.notConnectedToInternet)
        do {
            try await store.send(to: "u2", recordId: "r2", fromRecordId: "r1")
            XCTFail("must throw")
        } catch {}
        XCTAssertEqual(store.state(of: "u2"), .none, "送れなければ「つながりたい」に戻る")
        XCTAssertEqual(store.items(.outgoing), [])
    }

    /// 相手から届いている人に送る = 承認と同じ。サーバーの応答(matched)で確定する。
    func testSendToIncomingConnects() async throws {
        let fake = FakeMatchingClient()
        let store = try await loaded(fake)
        fake.sendResult = MatchInterest(uid: "u3", profileRecordId: "r3", status: "matched")
        nonisolated(unsafe) var during: InterestState?
        fake.probe = { await MainActor.run { during = store.state(of: "u3") } }
        let result = try await store.send(to: "u3", recordId: "r3", fromRecordId: "r1")
        XCTAssertTrue(result.isMatched)
        XCTAssertEqual(during, .matched)
        XCTAssertEqual(store.items(.incoming), [])
        XCTAssertEqual(store.items(.matches)?.map(\.uid), ["u3", "u4"])
    }

    func testAcceptMovesToMatchesAndRollsBack() async throws {
        let fake = FakeMatchingClient()
        let store = try await loaded(fake)
        try await store.accept("u3")
        XCTAssertEqual(store.state(of: "u3"), .matched)
        XCTAssertEqual(fake.calls.last, "accept u3 age=true")

        let failing = FakeMatchingClient()
        let other = try await loaded(failing)
        failing.error = APIError.server(status: 404, code: "not_found", message: "no such interest")
        do { try await other.accept("u3"); XCTFail("must throw") } catch {}
        XCTAssertEqual(other.state(of: "u3"), .incoming, "失敗したら届いたままに戻る")
        XCTAssertEqual(other.items(.matches)?.map(\.uid), ["u4"])
    }

    func testDeclineWithdrawUnmatchAndRollback() async throws {
        let fake = FakeMatchingClient()
        let store = try await loaded(fake)
        try await store.send(to: "u2", recordId: "r2", fromRecordId: "r1")
        try await store.withdraw("u2")
        try await store.decline("u3")
        XCTAssertEqual(store.state(of: "u2"), .none)
        XCTAssertEqual(store.state(of: "u3"), .none)
        XCTAssertEqual(Array(fake.calls.suffix(2)), ["decline u2", "decline u3"], "見送る・取り下げるは同じ DELETE")

        fake.error = URLError(.timedOut)
        do { try await store.unmatch("u4"); XCTFail("must throw") } catch {}
        XCTAssertEqual(store.state(of: "u4"), .matched, "解除できなければつながったまま")
        fake.error = nil
        try await store.unmatch("u4")
        XCTAssertEqual(store.items(.matches), [])
    }

    func testFailedLoadIsReportedOnlyWithoutItems() async throws {
        let fake = FakeMatchingClient()
        let store = MatchingStore(appId: "app1", client: fake)
        fake.error = URLError(.notConnectedToInternet)
        do { try await store.load(.matches); XCTFail("must throw") } catch {}
        XCTAssertTrue(store.failed.contains(.matches))
        XCTAssertNil(store.items(.matches))
        fake.error = nil
        try await store.load(.matches)
        XCTAssertFalse(store.failed.contains(.matches))
        XCTAssertEqual(store.items(.matches), [])
    }

    func testForgetRemovesEverywhere() async throws {
        let store = try await loaded(FakeMatchingClient())
        store.forget("u3")
        store.forget("u4")
        XCTAssertEqual(store.items(.incoming), [])
        XCTAssertEqual(store.items(.matches), [])
    }

    func testAgeGateRemembersOnlyAFlag() throws {
        let defaults = try XCTUnwrap(UserDefaults(suiteName: "MatchingAgeGateTests"))
        defaults.removePersistentDomain(forName: "MatchingAgeGateTests")
        let gate = MatchingAgeGate(defaults: defaults)
        XCTAssertFalse(gate.isConfirmed)
        gate.confirm()
        XCTAssertTrue(gate.isConfirmed)
        XCTAssertEqual(defaults.dictionaryRepresentation().keys.filter { $0.hasPrefix("matching") }, [MatchingAgeGate.key], "年齢・生年月日は保存しない")
        gate.reset()
        XCTAssertFalse(gate.isConfirmed)
    }
}
#endif
