// 審査用ビルド(APP_STORE_REVIEW・#245)ではバイナリから外す。
#if !APP_STORE_REVIEW
import Foundation
import MultipeerConnectivity
import NearbyInteraction
import MetamonDSL
import MetamonRenderer

/// すれ違い通信を始める時に渡すもの。
struct PassbyStart {
    let hello: NearbyPassby.Hello
    let config: NearbyPassby.Config
    let entity: Entity
    /// 送るデータ(つながった時に作る = 最新の記録)
    let payload: @MainActor () -> NearbyPassby.Payload
    /// この相手(senderId)とまだ交換してよいか(cooldown)
    let shouldExchange: @MainActor (String) -> Bool
}

/// すれ違い通信の通信路。実機は `MCPassbyTransport`、`METAMON_MOCK=1` は `MockPassbyTransport`。
@MainActor
protocol PassbyTransport: AnyObject {
    var onPayload: ((NearbyPassby.Payload) -> Void)? { get set }
    var onPeers: (([PassbyPeer]) -> Void)? { get set }
    var onNotice: ((String?) -> Void)? { get set }
    var distanceSupport: PassbyDistanceSupport { get }
    func start(_ start: PassbyStart)
    func stop()
}

/// MultipeerConnectivity で近くの同じアプリを探して自動で交換し、distance なら NearbyInteraction の token も交換して距離・方向を測る。
///
/// - 同じアプリかは discoveryInfo の `a`(appId のハッシュ)で判定。招待は senderId の小さい側だけが出す(二重接続を防ぐ)
/// - 相手ごとに暗号化必須の MCSession。交換が済んだら切る(distance の時は測り続けるためつないだまま)
/// - 受け取るデータは 1 通 32 KB まで(相手の上限 16 KB + 余裕)
@MainActor
final class MCPassbyTransport: NSObject, PassbyTransport {
    /// Info.plist `NSBonjourServices` の `_tailorit-pb._tcp` / `_udp` と一致させる(15 文字以内)
    static let serviceType = "tailorit-pb"
    static let maxMessageBytes = 32 * 1024

    var onPayload: ((NearbyPassby.Payload) -> Void)?
    var onPeers: (([PassbyPeer]) -> Void)?
    var onNotice: ((String?) -> Void)?

    var distanceSupport: PassbyDistanceSupport {
        NISession.deviceCapabilities.supportsPreciseDistanceMeasurement
            ? .supported : .unsupported("この端末は距離・方向の測定に対応していません(iPhone 11 以降・SE を除く)")
    }

    private struct Message: Codable {
        var payload: NearbyPassby.Payload?
        var niToken: Data?
    }

    private final class Link {
        let session: MCSession
        var senderId: String
        var name: String
        var sent = false
        var received = false
        var ni: NISession?
        var distance: Float?
        var azimuth: Float?
        init(session: MCSession, senderId: String, name: String) {
            self.session = session
            self.senderId = senderId
            self.name = name
        }
    }

    private var current: PassbyStart?
    private var me: MCPeerID?
    private var advertiser: MCNearbyServiceAdvertiser?
    private var browser: MCNearbyServiceBrowser?
    private var links: [MCPeerID: Link] = [:]

    func start(_ start: PassbyStart) {
        stop()
        current = start
        var name = start.hello.name
        while name.utf8.count > 63 { name.removeLast() }
        let peer = MCPeerID(displayName: name.isEmpty ? "Tailor It" : name)
        me = peer
        let info = ["v": "\(NearbyPassby.version)", "a": start.hello.app, "s": start.hello.senderId]
        let a = MCNearbyServiceAdvertiser(peer: peer, discoveryInfo: info, serviceType: Self.serviceType)
        a.delegate = self
        a.startAdvertisingPeer()
        advertiser = a
        let b = MCNearbyServiceBrowser(peer: peer, serviceType: Self.serviceType)
        b.delegate = self
        b.startBrowsingForPeers()
        browser = b
        onNotice?(nil)
    }

    func stop() {
        advertiser?.stopAdvertisingPeer()
        browser?.stopBrowsingForPeers()
        advertiser = nil
        browser = nil
        for link in links.values {
            link.ni?.invalidate()
            link.session.disconnect()
        }
        links = [:]
        current = nil
        publishPeers()
    }

    // MARK: - つなぐ

    private func makeLink(for peer: MCPeerID, senderId: String) -> Link? {
        guard let me else { return nil }
        let session = MCSession(peer: me, securityIdentity: nil, encryptionPreference: .required)
        session.delegate = self
        let link = Link(session: session, senderId: senderId, name: peer.displayName)
        links[peer] = link
        return link
    }

    private func found(_ peer: MCPeerID, info: [String: String]?) {
        guard let cur = current, let browser, links[peer] == nil,
              info?["a"] == cur.hello.app, let s = info?["s"], s != cur.hello.senderId,
              cur.hello.senderId < s, // 小さい側だけが招待する
              cur.config.distance || cur.shouldExchange(s),
              let context = try? NearbyPassby.encode(cur.hello),
              let link = makeLink(for: peer, senderId: s) else { return }
        browser.invitePeer(peer, to: link.session, withContext: context, timeout: 20)
    }

    private func invited(by peer: MCPeerID, context: Data?, handler: @escaping (Bool, MCSession?) -> Void) {
        guard let cur = current, links[peer] == nil,
              let hello = context.flatMap({ try? NearbyPassby.decode(NearbyPassby.Hello.self, from: $0) }),
              hello.app == cur.hello.app, hello.senderId != cur.hello.senderId,
              cur.config.distance || cur.shouldExchange(hello.senderId),
              let link = makeLink(for: peer, senderId: hello.senderId) else {
            handler(false, nil)
            return
        }
        link.name = hello.name
        handler(true, link.session)
    }

    private func connected(_ peer: MCPeerID) {
        guard let cur = current, let link = links[peer] else { return }
        send(Message(payload: cur.payload()), to: peer, link)
        link.sent = true
        if cur.config.distance, case .supported = distanceSupport {
            let ni = NISession()
            ni.delegate = self
            link.ni = ni
            if let token = ni.discoveryToken,
               let data = try? NSKeyedArchiver.archivedData(withRootObject: token, requiringSecureCoding: true) {
                send(Message(niToken: data), to: peer, link)
            }
        }
        publishPeers()
        finishIfDone(peer, link)
    }

    private func disconnected(_ peer: MCPeerID) {
        guard let link = links.removeValue(forKey: peer) else { return }
        link.ni?.invalidate()
        publishPeers()
    }

    private func received(_ data: Data, from peer: MCPeerID) {
        guard data.count <= Self.maxMessageBytes, let link = links[peer],
              let msg = try? NearbyPassby.decode(Message.self, from: data) else { return }
        if var payload = msg.payload, !link.received {
            link.received = true
            payload.senderId = link.senderId // 招待時の id に揃える(なりすまし防止)
            link.name = payload.name
            onPayload?(payload)
        }
        if let tokenData = msg.niToken, let ni = link.ni,
           let token = try? NSKeyedUnarchiver.unarchivedObject(ofClass: NIDiscoveryToken.self, from: tokenData) {
            ni.run(NINearbyPeerConfiguration(peerToken: token))
        }
        publishPeers()
        finishIfDone(peer, link)
    }

    /// 交換が済んだら切る(距離を測る時はつないだまま)。
    private func finishIfDone(_ peer: MCPeerID, _ link: Link) {
        guard link.sent, link.received, current?.config.distance != true else { return }
        Task { @MainActor [weak self] in
            try? await Task.sleep(for: .seconds(1))
            guard let self, self.links[peer] === link else { return }
            link.session.disconnect()
            self.links[peer] = nil
            self.publishPeers()
        }
    }

    private func send(_ msg: Message, to peer: MCPeerID, _ link: Link) {
        guard let data = try? NearbyPassby.encode(msg) else { return }
        try? link.session.send(data, toPeers: [peer], with: .reliable)
    }

    private func publishPeers() {
        let peers = links.values.filter { $0.session.connectedPeers.isEmpty == false }
            .map { PassbyPeer(id: $0.senderId, name: $0.name, distance: $0.distance, azimuth: $0.azimuth) }
            .sorted { ($0.distance ?? .infinity) < ($1.distance ?? .infinity) }
        onPeers?(peers)
    }

    private func updated(_ session: NISession, _ objects: [NINearbyObject]) {
        guard let link = links.values.first(where: { $0.ni === session }), let obj = objects.first else { return }
        link.distance = obj.distance
        if let dir = obj.direction {
            link.azimuth = asin(max(-1, min(1, dir.x)))
        } else if let h = obj.horizontalAngle {
            link.azimuth = h
        }
        publishPeers()
    }

    private func lostRange(_ session: NISession) {
        guard let link = links.values.first(where: { $0.ni === session }) else { return }
        link.distance = nil
        link.azimuth = nil
        publishPeers()
    }
}

// MARK: - delegates(任意スレッドから呼ばれる → MainActor へ)

extension MCPassbyTransport: MCNearbyServiceAdvertiserDelegate {
    nonisolated func advertiser(_ advertiser: MCNearbyServiceAdvertiser, didReceiveInvitationFromPeer peerID: MCPeerID,
                                withContext context: Data?, invitationHandler: @escaping (Bool, MCSession?) -> Void) {
        Task { @MainActor in self.invited(by: peerID, context: context, handler: invitationHandler) }
    }

    nonisolated func advertiser(_ advertiser: MCNearbyServiceAdvertiser, didNotStartAdvertisingPeer error: Error) {
        Task { @MainActor in self.onNotice?("近くの端末を探せませんでした(設定 › プライバシー › ローカルネットワークで Tailor It を許可してください)") }
    }
}

extension MCPassbyTransport: MCNearbyServiceBrowserDelegate {
    nonisolated func browser(_ browser: MCNearbyServiceBrowser, foundPeer peerID: MCPeerID, withDiscoveryInfo info: [String: String]?) {
        Task { @MainActor in self.found(peerID, info: info) }
    }

    nonisolated func browser(_ browser: MCNearbyServiceBrowser, lostPeer peerID: MCPeerID) {}

    nonisolated func browser(_ browser: MCNearbyServiceBrowser, didNotStartBrowsingForPeers error: Error) {
        Task { @MainActor in self.onNotice?("近くの端末を探せませんでした(設定 › プライバシー › ローカルネットワークで Tailor It を許可してください)") }
    }
}

extension MCPassbyTransport: MCSessionDelegate {
    nonisolated func session(_ session: MCSession, peer peerID: MCPeerID, didChange state: MCSessionState) {
        Task { @MainActor in
            switch state {
            case .connected: self.connected(peerID)
            case .notConnected: self.disconnected(peerID)
            default: break
            }
        }
    }

    nonisolated func session(_ session: MCSession, didReceive data: Data, fromPeer peerID: MCPeerID) {
        Task { @MainActor in self.received(data, from: peerID) }
    }

    nonisolated func session(_ session: MCSession, didReceive stream: InputStream, withName streamName: String, fromPeer peerID: MCPeerID) {}
    nonisolated func session(_ session: MCSession, didStartReceivingResourceWithName resourceName: String, fromPeer peerID: MCPeerID, with progress: Progress) {}
    nonisolated func session(_ session: MCSession, didFinishReceivingResourceWithName resourceName: String, fromPeer peerID: MCPeerID, at localURL: URL?, withError error: Error?) {}
}

extension MCPassbyTransport: NISessionDelegate {
    nonisolated func session(_ session: NISession, didUpdate nearbyObjects: [NINearbyObject]) {
        Task { @MainActor in self.updated(session, nearbyObjects) }
    }

    nonisolated func session(_ session: NISession, didRemove nearbyObjects: [NINearbyObject], reason: NINearbyObject.RemovalReason) {
        Task { @MainActor in self.lostRange(session) }
    }

    nonisolated func sessionWasSuspended(_ session: NISession) {
        Task { @MainActor in self.lostRange(session) }
    }

    nonisolated func session(_ session: NISession, didInvalidateWith error: Error) {
        Task { @MainActor in self.lostRange(session) }
    }
}
#endif
