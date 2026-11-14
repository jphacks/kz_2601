#if !APP_STORE_REVIEW // 審査用ビルドは通話(#447)を外す
import Foundation
import LiveKit

/// `CallRoom` の本物(LiveKit Cloud の部屋)。音声・映像の中継は LiveKit がする。着信・バックグラウンドの通話はしない。
@MainActor
public final class LiveKitCallRoom: CallRoom {
    public var onEvent: ((CallRoomEvent) -> Void)?
    private let room: Room
    private let relay = Relay()
    private var views: [String: VideoView] = [:]

    public init() {
        room = Room()
        relay.owner = self
        room.add(delegate: relay)
    }

    public var participants: [CallParticipant] {
        let me = room.localParticipant
        var out = [Self.participant(me, isLocal: true)]
        let others = room.remoteParticipants.values.map { Self.participant($0, isLocal: false) }
        out += others.sorted { $0.name.localizedStandardCompare($1.name) == .orderedAscending }
        return out
    }

    private static func participant(_ p: Participant, isLocal: Bool) -> CallParticipant {
        let id = p.identity?.stringValue ?? ""
        return CallParticipant(id: id, name: p.name ?? "", isLocal: isLocal, isSpeaking: p.isSpeaking,
                               micOn: p.isMicrophoneEnabled(), cameraOn: p.isCameraEnabled())
    }

    public func connect(url: String, token: String) async throws {
        try await room.connect(url: url, token: token)
    }

    public func disconnect() async {
        await room.disconnect()
        views = [:]
    }

    public func setMicrophone(_ on: Bool) async throws {
        try await room.localParticipant.setMicrophone(enabled: on)
    }

    public func setCamera(_ on: Bool) async throws {
        try await room.localParticipant.setCamera(enabled: on)
    }

    public func videoView(for participantID: String) -> AnyObject? {
        let all: [Participant] = [room.localParticipant] + Array(room.remoteParticipants.values)
        guard let p = all.first(where: { $0.identity?.stringValue == participantID }), let track = p.firstCameraVideoTrack else {
            views[participantID] = nil
            return nil
        }
        let view = views[participantID] ?? VideoView()
        view.layoutMode = .fill
        view.track = track
        views[participantID] = view
        return view
    }

    fileprivate func notify(_ event: CallRoomEvent) { onEvent?(event) }
}

/// LiveKit の delegate(どのスレッドからも来る)を MainActor に渡す。
private final class Relay: NSObject, RoomDelegate, @unchecked Sendable {
    @MainActor weak var owner: LiveKitCallRoom?

    private func send(_ event: CallRoomEvent) {
        Task { @MainActor [weak self] in self?.owner?.notify(event) }
    }

    func room(_ room: Room, participantDidConnect participant: RemoteParticipant) { send(.changed) }
    func room(_ room: Room, participantDidDisconnect participant: RemoteParticipant) { send(.changed) }
    func room(_ room: Room, didUpdateSpeakingParticipants participants: [Participant]) { send(.changed) }
    func room(_ room: Room, participant: Participant, trackPublication: TrackPublication, didUpdateIsMuted isMuted: Bool) { send(.changed) }
    func room(_ room: Room, participant: RemoteParticipant, didSubscribeTrack publication: RemoteTrackPublication) { send(.changed) }
    func room(_ room: Room, participant: RemoteParticipant, didUnsubscribeTrack publication: RemoteTrackPublication) { send(.changed) }
    func room(_ room: Room, participant: LocalParticipant, didPublishTrack publication: LocalTrackPublication) { send(.changed) }
    func room(_ room: Room, participant: LocalParticipant, didUnpublishTrack publication: LocalTrackPublication) { send(.changed) }
    func room(_ room: Room, didDisconnectWithError error: LiveKitError?) { send(.disconnected) }
}
#endif
