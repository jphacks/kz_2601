#if !APP_STORE_REVIEW
import XCTest
import MetamonDSL
@testable import MetamonServices

/// 通話(#447)の流れ: 入る・ミュート・カメラ・押さえ直し・コイン不足・出る。LiveKit にはつながない(偽の部屋)。
@MainActor
final class CallSessionTests: XCTestCase {
    private let t0 = Date(timeIntervalSince1970: 1_800_000_000)

    func testJoinConnectsWithMicAndCameraWhenGranted() async {
        let client = FakeCallClient(results: [.success(join(video: true))])
        let room = FakeCallRoom()
        let s = session(client: client, room: room, video: true)
        await s.join()
        XCTAssertEqual(s.phase, .connected)
        XCTAssertEqual(room.connected, ["wss://example.livekit.cloud|tok-1"])
        XCTAssertEqual(room.mic, [true])
        XCTAssertEqual(room.camera, [true])
        XCTAssertTrue(s.videoGranted)
        XCTAssertTrue(s.cameraOn)
        XCTAssertEqual(client.requests, [CallJoinRequest(room: "lobby", video: true, name: "ゆう")])
        XCTAssertEqual(s.participants.map(\.id), ["me"])
    }

    func testAudioRoomKeepsCameraOff() async {
        let room = FakeCallRoom()
        let s = session(client: FakeCallClient(results: [.success(join(video: false))]), room: room, video: true)
        await s.join()
        XCTAssertEqual(room.camera, [], "サーバーがカメラを許さなければ入れない")
        await s.toggleCamera()
        XCTAssertEqual(room.camera, [])
        XCTAssertFalse(s.cameraOn)
    }

    func testMuteAndCameraToggle() async {
        let room = FakeCallRoom()
        let s = session(client: FakeCallClient(results: [.success(join(video: true))]), room: room, video: true)
        await s.join()
        await s.toggleMic()
        XCTAssertFalse(s.micOn)
        await s.toggleCamera()
        XCTAssertFalse(s.cameraOn)
        XCTAssertEqual(room.mic, [true, false])
        XCTAssertEqual(room.camera, [true, false])
        room.failNext = true
        await s.toggleMic()
        XCTAssertFalse(s.micOn, "失敗したら表示を変えない")
    }

    func testCoinsInsufficientOnJoin() async {
        let room = FakeCallRoom()
        let s = session(client: FakeCallClient(results: [.failure(Self.coins)]), room: room)
        await s.join()
        XCTAssertEqual(s.phase, .ended(.coins(required: 30, balance: 12)))
        XCTAssertEqual(room.connected, [], "トークンが無ければつながない")
    }

    func testDenials() async {
        let cases: [(APIError, CallSession.EndReason)] = [
            (.server(status: 404, code: "not_found", message: "", details: ["reason": .string("calls_disabled")]), .denied(.disabled)),
            (.server(status: 403, code: "forbidden", message: "", details: ["reason": .string("not_member")]), .denied(.notMember)),
            (.server(status: 429, code: "rate_limited", message: ""), .denied(.rateLimited)),
            (.http(status: 500, body: ""), .failed),
        ]
        for (error, want) in cases {
            let s = session(client: FakeCallClient(results: [.failure(error)]), room: FakeCallRoom())
            await s.join()
            XCTAssertEqual(s.phase, .ended(want), "\(error)")
        }
    }

    func testConnectFailureDisconnects() async {
        let room = FakeCallRoom()
        room.failConnect = true
        let s = session(client: FakeCallClient(results: [.success(join())]), room: room)
        await s.join()
        XCTAssertEqual(s.phase, .ended(.failed))
        XCTAssertEqual(room.disconnects, 1)
    }

    func testRenewSendsSessionAndStopsOnCoins() async {
        let client = FakeCallClient(results: [.success(join()), .success(join(until: 3600)), .failure(Self.coins)])
        let room = FakeCallRoom()
        let s = session(client: client, room: room, sleep: { _ in try await Task.sleep(nanoseconds: 60_000_000_000) })
        await s.join()
        let next = await s.renew()
        XCTAssertEqual(next, t0.addingTimeInterval(3600))
        XCTAssertEqual(client.requests.last?.sessionId, "s1", "続ける時は同じ sessionId")
        XCTAssertEqual(s.phase, .connected)
        let none = await s.renew()
        XCTAssertNil(none)
        XCTAssertEqual(s.phase, .ended(.coins(required: 30, balance: 12)), "コインが足りなければ出る")
        XCTAssertEqual(room.disconnects, 1)
        XCTAssertTrue(s.participants.isEmpty)
    }

    func testRenewalIsScheduledBeforeTheBlockEnds() async {
        let client = FakeCallClient(results: [.success(join(until: 1800)), .success(join(until: 3600)), .failure(Self.coins)])
        let waits = Waits()
        let s = session(client: client, room: FakeCallRoom(), sleep: { await waits.add($0) })
        await s.join()
        for _ in 0..<200 where client.requests.count < 3 { await Task.yield() }
        XCTAssertEqual(client.requests.count, 3)
        let got = await waits.all
        XCTAssertEqual(got.first, 1800 - CallSession.renewLead, "切れる 1 分前に押さえ直す")
        XCTAssertEqual(s.phase, .ended(.coins(required: 30, balance: 12)))
    }

    func testLeaveAndServerDisconnect() async {
        let room = FakeCallRoom()
        let s = session(client: FakeCallClient(results: [.success(join())]), room: room)
        await s.join()
        room.others = [CallParticipant(id: "p2", name: "ママ", isLocal: false)]
        room.onEvent?(.changed)
        XCTAssertEqual(s.participants.map(\.name), ["ゆう", "ママ"])
        await s.leave()
        await s.leave()
        XCTAssertEqual(s.phase, .ended(.left))
        XCTAssertEqual(room.disconnects, 1, "2 回目の退出は何もしない")

        let room2 = FakeCallRoom()
        let s2 = session(client: FakeCallClient(results: [.success(join())]), room: room2)
        await s2.join()
        room2.onEvent?(.disconnected)
        for _ in 0..<50 where s2.phase == .connected { await Task.yield() }
        XCTAssertEqual(s2.phase, .ended(.failed))
    }

    func testDecodesTheBackendResponse() throws {
        let body = #"{"url":"wss://x.livekit.cloud","token":"t","room":"app:a1","identity":"s1","sessionId":"s1","expiresAt":"2026-11-10T12:10:00Z","video":false,"blockMinutes":30,"reservedUntil":"2026-11-10T12:30:00Z","coinsReserved":60,"coinsPerMinute":2}"#
        let j = try HTTPCallClient.decoder.decode(CallJoin.self, from: Data(body.utf8))
        XCTAssertEqual(j.sessionId, "s1")
        XCTAssertEqual(j.coinsPerMinute, 2)
        XCTAssertEqual(j.reservedUntil.timeIntervalSince(j.expiresAt), 20 * 60)
    }

    // MARK: - helpers

    private static let coins = APIError.server(status: 402, code: "coins_insufficient", message: "not enough coins",
                                               details: ["required": .number(30), "balance": .number(12), "payer": .string("self")])

    private func join(video: Bool = false, until: TimeInterval = 1800) -> CallJoin {
        CallJoin(url: "wss://example.livekit.cloud", token: "tok-1", room: "app:a1:lobby", identity: "s1", sessionId: "s1",
                 expiresAt: t0.addingTimeInterval(600), video: video, blockMinutes: 30, reservedUntil: t0.addingTimeInterval(until))
    }

    private func session(client: FakeCallClient, room: FakeCallRoom, video: Bool = false,
                         sleep: @escaping @Sendable (TimeInterval) async throws -> Void = { _ in throw CancellationError() }) -> CallSession {
        let t0 = self.t0
        return CallSession(appId: "a1", roomKey: "lobby", video: video, name: "ゆう", client: client, room: room, now: { t0 }, sleep: sleep)
    }
}

private actor Waits {
    var all: [TimeInterval] = []
    func add(_ w: TimeInterval) { all.append(w) }
}

private final class FakeCallClient: CallClient, @unchecked Sendable {
    private var results: [Result<CallJoin, Error>]
    private(set) var requests: [CallJoinRequest] = []
    private let lock = NSLock()

    init(results: [Result<CallJoin, Error>]) { self.results = results }

    func join(appId: String, request: CallJoinRequest) async throws -> CallJoin {
        let next: Result<CallJoin, Error> = lock.withLock {
            requests.append(request)
            return results.isEmpty ? .failure(APIError.http(status: 500, body: "")) : results.removeFirst()
        }
        return try next.get()
    }
}

@MainActor
private final class FakeCallRoom: CallRoom {
    var onEvent: ((CallRoomEvent) -> Void)?
    var others: [CallParticipant] = []
    var connected: [String] = []
    var mic: [Bool] = []
    var camera: [Bool] = []
    var disconnects = 0
    var failConnect = false
    var failNext = false

    var participants: [CallParticipant] { [CallParticipant(id: "me", name: "ゆう", isLocal: true)] + others }

    func connect(url: String, token: String) async throws {
        if failConnect { throw URLError(.cannotConnectToHost) }
        connected.append("\(url)|\(token)")
    }

    func disconnect() async { disconnects += 1 }

    func setMicrophone(_ on: Bool) async throws {
        if failNext { failNext = false; throw URLError(.unknown) }
        mic.append(on)
    }

    func setCamera(_ on: Bool) async throws {
        if failNext { failNext = false; throw URLError(.unknown) }
        camera.append(on)
    }

    func videoView(for participantID: String) -> AnyObject? { nil }
}
#endif
