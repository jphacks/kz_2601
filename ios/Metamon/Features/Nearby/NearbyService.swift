// 審査用ビルド(APP_STORE_REVIEW・#245)ではバイナリから外す。
#if !APP_STORE_REVIEW
import Foundation
import MultipeerConnectivity
import Observation
import MetamonDSL

/// `nearbyShare`(#36)の通信。MultipeerConnectivity(Wi-Fi / Bluetooth を OS が自動選択・インターネット不要)。
///
/// - 受信: 前面にいる間だけ advertise。招待の context に載った `NearbyShare.Header` を見て確認を出し、承認した時だけ接続する
/// - 送信: シートを開いている間だけ browse。相手を選ぶと Header 付きで招待 → 接続できたら Payload を 1 回送り、Ack を待つ
/// - セッションは暗号化必須(`.required`)。1 回の受け渡しが終わったら切断する
@MainActor
@Observable
final class NearbyService: NSObject {
    /// Bonjour のサービス名(Info.plist `NSBonjourServices` の `_tailorit-mc._tcp` と一致させる。15 文字以内)
    static let serviceType = "tailorit-mc"

    struct Peer: Identifiable, Hashable {
        let id: MCPeerID
        var name: String { id.displayName }
    }

    enum SendState: Equatable {
        case idle
        case inviting(String)
        case sending(String)
        case done(String)
        case failed(String)

        var isBusy: Bool {
            switch self { case .inviting, .sending: true; default: false }
        }
    }

    /// 承認待ちの招待(RootView が確認ダイアログを出す)。
    struct Incoming: Identifiable {
        let id = UUID()
        let header: NearbyShare.Header
        let decision: NearbyShare.Decision
        fileprivate let peer: MCPeerID
        fileprivate let respond: (Bool, MCSession?) -> Void
    }

    private(set) var peers: [Peer] = []
    private(set) var sendState: SendState = .idle
    var incoming: Incoming?

    /// 受け取ったデータを取り込む(AppRuntime が設定)。戻り値は送信側に返す Ack。
    @ObservationIgnored var onReceive: (@MainActor (NearbyShare.Payload, NearbyShare.Decision) async -> NearbyShare.Ack)?
    /// 招待時の取り込み判定(AppRuntime が設定・今開いているアプリで判定)。
    @ObservationIgnored var decide: (@MainActor (NearbyShare.Header) -> NearbyShare.Decision)?

    @ObservationIgnored private let me: MCPeerID
    @ObservationIgnored private var session: MCSession
    @ObservationIgnored private var advertiser: MCNearbyServiceAdvertiser?
    @ObservationIgnored private var browser: MCNearbyServiceBrowser?
    /// 送信中: 相手と送るデータ
    @ObservationIgnored private var outgoing: (peer: MCPeerID, data: Data)?
    /// 受信中: 承認済みの相手 → 判定
    @ObservationIgnored private var accepted: [MCPeerID: NearbyShare.Decision] = [:]
    @ObservationIgnored private var timeoutTask: Task<Void, Never>?

    init(displayName: String) {
        // MCPeerID の displayName は UTF-8 で 63 バイトまで
        var name = displayName.trimmingCharacters(in: .whitespacesAndNewlines)
        if name.isEmpty { name = "Tailor It" }
        while name.utf8.count > 63 { name.removeLast() }
        me = MCPeerID(displayName: name)
        session = MCSession(peer: me, securityIdentity: nil, encryptionPreference: .required)
        super.init()
        session.delegate = self
    }

    var myName: String { me.displayName }

    // MARK: - 受信(advertise)

    func setAdvertising(_ on: Bool) {
        if on {
            guard advertiser == nil else { return }
            let a = MCNearbyServiceAdvertiser(peer: me, discoveryInfo: ["v": "\(NearbyShare.version)"], serviceType: Self.serviceType)
            a.delegate = self
            a.startAdvertisingPeer()
            advertiser = a
        } else {
            advertiser?.stopAdvertisingPeer()
            advertiser = nil
            if let pending = incoming { pending.respond(false, nil); incoming = nil }
        }
    }

    /// 確認ダイアログの結果。拒否・取り込めない時は招待を断る(勝手に受け取らない)。
    func answer(_ item: Incoming, accept: Bool) {
        incoming = nil
        let ok = accept && { if case .accept = item.decision { return true }; return false }()
        if ok { accepted[item.peer] = item.decision }
        item.respond(ok, ok ? session : nil)
    }

    // MARK: - 送信(browse)

    func startBrowsing() {
        peers = []
        sendState = .idle
        guard browser == nil else { return }
        let b = MCNearbyServiceBrowser(peer: me, serviceType: Self.serviceType)
        b.delegate = self
        b.startBrowsingForPeers()
        browser = b
    }

    func stopBrowsing() {
        browser?.stopBrowsingForPeers()
        browser = nil
        peers = []
        if sendState.isBusy { finishOutgoing(.idle) }
    }

    func send(_ payload: NearbyShare.Payload, to peer: Peer) {
        guard !sendState.isBusy, let browser else { return }
        do {
            let data = try NearbyShare.encode(payload)
            let context = try NearbyShare.encode(payload.header)
            outgoing = (peer.id, data)
            sendState = .inviting(peer.name)
            browser.invitePeer(peer.id, to: session, withContext: context, timeout: 30)
            timeoutTask?.cancel()
            timeoutTask = Task { [weak self] in
                try? await Task.sleep(for: .seconds(35))
                guard let self, self.sendState.isBusy else { return }
                self.finishOutgoing(.failed("\(peer.name)さんから応答がありませんでした"))
            }
        } catch {
            sendState = .failed("送るデータを作れませんでした")
        }
    }

    private func finishOutgoing(_ state: SendState) {
        timeoutTask?.cancel()
        outgoing = nil
        sendState = state
        session.disconnect()
    }

    // MARK: - セッションイベント(MainActor に寄せてから処理)

    private func peerConnected(_ peer: MCPeerID) {
        guard let out = outgoing, out.peer == peer else { return }
        sendState = .sending(peer.displayName)
        do {
            try session.send(out.data, toPeers: [peer], with: .reliable)
        } catch {
            finishOutgoing(.failed("送信に失敗しました"))
        }
    }

    private func peerDisconnected(_ peer: MCPeerID) {
        accepted[peer] = nil
        if let out = outgoing, out.peer == peer, sendState.isBusy {
            finishOutgoing(.failed(sendState == .inviting(peer.displayName)
                ? "\(peer.displayName)さんが受け取りませんでした" : "接続が切れました"))
        }
    }

    private func received(_ data: Data, from peer: MCPeerID) async {
        // 送信側: Ack を受け取ったら完了
        if let out = outgoing, out.peer == peer, let ack = try? NearbyShare.decode(NearbyShare.Ack.self, from: data) {
            finishOutgoing(ack.ok
                ? .done("\(peer.displayName)さんに \(ack.added) 件届きました")
                : .failed(ack.message ?? "\(peer.displayName)さんが受け取れませんでした"))
            return
        }
        // 受信側: 承認した相手からの Payload だけ取り込む
        guard let decision = accepted.removeValue(forKey: peer) else { return }
        let ack: NearbyShare.Ack
        if let payload = try? NearbyShare.decode(NearbyShare.Payload.self, from: data), let onReceive {
            ack = await onReceive(payload, decision)
        } else {
            ack = .init(ok: false, added: 0, message: "データを読み取れませんでした")
        }
        if let reply = try? NearbyShare.encode(ack) {
            try? session.send(reply, toPeers: [peer], with: .reliable)
        }
    }
}

// MARK: - MultipeerConnectivity delegates(任意スレッドから呼ばれる)

extension NearbyService: MCNearbyServiceAdvertiserDelegate {
    nonisolated func advertiser(_ advertiser: MCNearbyServiceAdvertiser, didReceiveInvitationFromPeer peerID: MCPeerID,
                                withContext context: Data?, invitationHandler: @escaping (Bool, MCSession?) -> Void) {
        let header = context.flatMap { try? NearbyShare.decode(NearbyShare.Header.self, from: $0) }
        Task { @MainActor in
            // Header が無い・読めない招待(Tailor It 以外 / 壊れたデータ)と、確認中の二重招待は黙って断る
            guard let header, self.incoming == nil else { invitationHandler(false, nil); return }
            let decision = self.decide?(header) ?? .reject(reason: "今は受け取れません")
            self.incoming = Incoming(header: header, decision: decision, peer: peerID, respond: invitationHandler)
        }
    }

    nonisolated func advertiser(_ advertiser: MCNearbyServiceAdvertiser, didNotStartAdvertisingPeer error: Error) {
        print("[Metamon] nearby: advertise failed \(error)")
    }
}

extension NearbyService: MCNearbyServiceBrowserDelegate {
    nonisolated func browser(_ browser: MCNearbyServiceBrowser, foundPeer peerID: MCPeerID, withDiscoveryInfo info: [String: String]?) {
        Task { @MainActor in
            guard !self.peers.contains(where: { $0.id == peerID }) else { return }
            self.peers.append(Peer(id: peerID))
        }
    }

    nonisolated func browser(_ browser: MCNearbyServiceBrowser, lostPeer peerID: MCPeerID) {
        Task { @MainActor in self.peers.removeAll { $0.id == peerID } }
    }

    nonisolated func browser(_ browser: MCNearbyServiceBrowser, didNotStartBrowsingForPeers error: Error) {
        Task { @MainActor in self.sendState = .failed("近くの端末を探せませんでした(ローカルネットワークの許可を確認してください)") }
    }
}

extension NearbyService: MCSessionDelegate {
    nonisolated func session(_ session: MCSession, peer peerID: MCPeerID, didChange state: MCSessionState) {
        Task { @MainActor in
            switch state {
            case .connected: self.peerConnected(peerID)
            case .notConnected: self.peerDisconnected(peerID)
            default: break
            }
        }
    }

    nonisolated func session(_ session: MCSession, didReceive data: Data, fromPeer peerID: MCPeerID) {
        Task { @MainActor in await self.received(data, from: peerID) }
    }

    nonisolated func session(_ session: MCSession, didReceive stream: InputStream, withName streamName: String, fromPeer peerID: MCPeerID) {}
    nonisolated func session(_ session: MCSession, didStartReceivingResourceWithName resourceName: String, fromPeer peerID: MCPeerID, with progress: Progress) {}
    nonisolated func session(_ session: MCSession, didFinishReceivingResourceWithName resourceName: String, fromPeer peerID: MCPeerID, at localURL: URL?, withError error: Error?) {}
}
#endif
