#if !APP_STORE_REVIEW // 審査用ビルドは通話(#447)を外す(共有と一緒・審査メモの説明が要る)
import Foundation
import Observation
import MetamonDSL

/// `POST /v1/apps/{appId}/calls/token` の body(docs/backend/openapi-appconfig.yaml・#447)。
public struct CallJoinRequest: Encodable, Sendable, Equatable {
    public var room: String?
    public var video: Bool
    public var name: String?
    /// 続けて話す時(次のまとまりを押さえる)。
    public var sessionId: String?

    public init(room: String?, video: Bool, name: String?, sessionId: String? = nil) {
        self.room = room
        self.video = video
        self.name = name
        self.sessionId = sessionId
    }
}

/// 部屋に入るトークン。`token` はログ・画面に出さない。
public struct CallJoin: Decodable, Sendable, Equatable {
    public var url: String
    public var token: String
    public var room: String
    public var identity: String
    public var sessionId: String
    public var expiresAt: Date
    public var video: Bool
    public var blockMinutes: Int
    /// この時刻の前に sessionId でもう一度呼ぶ。
    public var reservedUntil: Date
    public var coinsReserved: Int
    public var coinsPerMinute: Int

    public init(url: String, token: String, room: String, identity: String, sessionId: String, expiresAt: Date, video: Bool,
                blockMinutes: Int, reservedUntil: Date, coinsReserved: Int = 0, coinsPerMinute: Int = 0) {
        self.url = url
        self.token = token
        self.room = room
        self.identity = identity
        self.sessionId = sessionId
        self.expiresAt = expiresAt
        self.video = video
        self.blockMinutes = blockMinutes
        self.reservedUntil = reservedUntil
        self.coinsReserved = coinsReserved
        self.coinsPerMinute = coinsPerMinute
    }
}

/// appconfig の通話の API。
public protocol CallClient: Sendable {
    func join(appId: String, request: CallJoinRequest) async throws -> CallJoin
}

/// 通話の API が断った理由(画面の文言と、コインの購入画面へ進むかを決める)。
public enum CallDenial: Equatable, Sendable {
    /// 402 `coins_insufficient`(自分のコイン)。コインの購入画面へ
    case coinsInsufficient(required: Int, balance: Int)
    /// 404 `calls_disabled`(サーバーで止めている)
    case disabled
    /// 403 `not_member`(参加していない)
    case notMember
    /// 404 `call_ended`(続けようとした通話が終わっている)
    case ended
    /// 404 その部屋の callButton が無い・共有でない
    case unavailable
    /// 429
    case rateLimited
    case other

    public init(_ error: Error) {
        guard let api = error as? APIError, case .server(let status, let code, _, let details) = api else {
            self = .other
            return
        }
        let reason = details["reason"]?.stringValue
        switch (status, code, reason) {
        case (402, "coins_insufficient", _):
            self = .coinsInsufficient(required: Int(details["required"]?.doubleValue ?? 0), balance: Int(details["balance"]?.doubleValue ?? 0))
        case (404, _, "calls_disabled"): self = .disabled
        case (404, _, "call_ended"): self = .ended
        case (404, _, _): self = .unavailable
        case (403, _, _): self = .notMember
        case (429, _, _): self = .rateLimited
        default: self = .other
        }
    }
}

/// 部屋の 1 人(タイルに出す)。
public struct CallParticipant: Identifiable, Equatable, Sendable {
    public var id: String
    public var name: String
    public var isLocal: Bool
    public var isSpeaking: Bool
    public var micOn: Bool
    public var cameraOn: Bool

    public init(id: String, name: String, isLocal: Bool, isSpeaking: Bool = false, micOn: Bool = true, cameraOn: Bool = false) {
        self.id = id
        self.name = name
        self.isLocal = isLocal
        self.isSpeaking = isSpeaking
        self.micOn = micOn
        self.cameraOn = cameraOn
    }
}

/// 部屋の中の変化(参加者・音声・映像 / 切れた)。
public enum CallRoomEvent: Sendable {
    case changed
    case disconnected
}

/// LiveKit の部屋の薄い窓口(テストでは偽物に差し替える)。
@MainActor
public protocol CallRoom: AnyObject {
    var onEvent: ((CallRoomEvent) -> Void)? { get set }
    var participants: [CallParticipant] { get }
    func connect(url: String, token: String) async throws
    func disconnect() async
    func setMicrophone(_ on: Bool) async throws
    func setCamera(_ on: Bool) async throws
    /// 映像のタイル(iOS では UIView)。映像が無ければ nil。
    func videoView(for participantID: String) -> AnyObject?
}

/// 1 回の通話の流れ(入る・ミュート・カメラ・まとまりの押さえ直し・出る)。画面(CallView)はこれを見るだけ。
@MainActor
@Observable
public final class CallSession {
    public enum EndReason: Equatable, Sendable {
        case left
        /// コインが足りない(入る時・続ける時)。コインの購入画面へ
        case coins(required: Int, balance: Int)
        case denied(CallDenial)
        /// つながらなかった・切れた
        case failed
    }

    public enum Phase: Equatable, Sendable {
        case idle, joining, connected
        case ended(EndReason)
    }

    public private(set) var phase: Phase = .idle
    public private(set) var participants: [CallParticipant] = []
    public private(set) var micOn = true
    public private(set) var cameraOn = false
    /// サーバーがカメラを許した(部品が video の時だけ)
    public private(set) var videoGranted = false
    public private(set) var coinsPerMinute = 0

    @ObservationIgnored public let room: any CallRoom
    @ObservationIgnored private let client: any CallClient
    @ObservationIgnored private let appId: String
    @ObservationIgnored private let roomKey: String?
    @ObservationIgnored private let wantsVideo: Bool
    @ObservationIgnored private let name: String?
    @ObservationIgnored private let now: () -> Date
    @ObservationIgnored private let sleep: @Sendable (TimeInterval) async throws -> Void
    @ObservationIgnored private(set) var sessionId: String?
    @ObservationIgnored private var renewTask: Task<Void, Never>?

    /// 押さえの切れる何秒前に押さえ直すか。
    public static let renewLead: TimeInterval = 60

    public init(appId: String, roomKey: String?, video: Bool, name: String?, client: any CallClient, room: any CallRoom,
                now: @escaping () -> Date = Date.init,
                sleep: @escaping @Sendable (TimeInterval) async throws -> Void = { try await Task.sleep(nanoseconds: UInt64(max(0, $0) * 1_000_000_000)) }) {
        self.appId = appId
        self.roomKey = roomKey
        self.wantsVideo = video
        self.name = name
        self.client = client
        self.room = room
        self.now = now
        self.sleep = sleep
    }

    /// トークンをもらって入る。マイクは入れて、ビデオの部屋ならカメラも入れる。
    public func join() async {
        guard phase == .idle else { return }
        phase = .joining
        let j: CallJoin
        do {
            j = try await client.join(appId: appId, request: CallJoinRequest(room: roomKey, video: wantsVideo, name: name))
        } catch {
            phase = .ended(Self.reason(for: error))
            return
        }
        sessionId = j.sessionId
        videoGranted = j.video
        coinsPerMinute = j.coinsPerMinute
        room.onEvent = { [weak self] event in self?.handle(event) }
        do {
            try await room.connect(url: j.url, token: j.token)
            try await room.setMicrophone(true)
            micOn = true
            if videoGranted {
                try await room.setCamera(true)
                cameraOn = true
            }
        } catch {
            await room.disconnect()
            phase = .ended(.failed)
            return
        }
        guard phase == .joining else { return } // 入っている間に閉じられた
        phase = .connected
        participants = room.participants
        scheduleRenewal(until: j.reservedUntil)
    }

    public func toggleMic() async {
        guard phase == .connected else { return }
        let next = !micOn
        do {
            try await room.setMicrophone(next)
            micOn = next
        } catch {}
    }

    public func toggleCamera() async {
        guard phase == .connected, videoGranted else { return }
        let next = !cameraOn
        do {
            try await room.setCamera(next)
            cameraOn = next
        } catch {}
    }

    /// 出る(自分で閉じた)。
    public func leave() async { await end(.left) }

    private func end(_ reason: EndReason) async {
        if case .ended = phase { return }
        renewTask?.cancel()
        renewTask = nil
        phase = .ended(reason)
        room.onEvent = nil
        await room.disconnect()
        participants = []
    }

    private func handle(_ event: CallRoomEvent) {
        switch event {
        case .changed:
            if phase == .connected { participants = room.participants }
        case .disconnected:
            Task { await end(.failed) }
        }
    }

    /// 押さえの切れる前に次のまとまりを押さえる。続けられなければ出る。
    private func scheduleRenewal(until first: Date) {
        renewTask?.cancel()
        renewTask = Task { [weak self] in
            var until = first
            while !Task.isCancelled {
                guard let self else { return }
                let wait = max(5, until.timeIntervalSince(self.now()) - Self.renewLead)
                do { try await self.sleep(wait) } catch { return }
                guard !Task.isCancelled, let next = await self.renew() else { return }
                until = next
            }
        }
    }

    /// 次のまとまりを押さえる。返り値は次の期限(nil = 続けられず出た)。
    func renew() async -> Date? {
        guard phase == .connected, let sessionId else { return nil }
        do {
            let j = try await client.join(appId: appId, request: CallJoinRequest(room: roomKey, video: wantsVideo, name: name, sessionId: sessionId))
            return j.reservedUntil
        } catch {
            await end(Self.reason(for: error))
            return nil
        }
    }

    static func reason(for error: Error) -> EndReason {
        switch CallDenial(error) {
        case .coinsInsufficient(let required, let balance): .coins(required: required, balance: balance)
        case .other: .failed
        case let denial: .denied(denial)
        }
    }
}

/// 通話(#447)。appconfig が live の時だけ(モックでは nil = 部品は「使えません」)。
public enum CallServiceKey: ServiceKey {
    public static var defaultValue: (any CallClient)? { nil }
}

extension Services {
    public var calls: (any CallClient)? {
        get { self[CallServiceKey.self] }
        set { self[CallServiceKey.self] = newValue }
    }
}
#endif
