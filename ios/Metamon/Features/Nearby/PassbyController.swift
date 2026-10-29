// 審査用ビルド(APP_STORE_REVIEW・#245)ではバイナリから外す。
#if !APP_STORE_REVIEW
import Foundation
import Observation
import MetamonDSL
import MetamonRenderer

/// すれ違い通信(#425)の本体側。`NearbyPassbyLeaf` のスイッチ・同意・会った人の一覧と、通信路(`PassbyTransport`)・記録の保存を結ぶ。
///
/// 端末に保存するもの(UserDefaults・サーバーへは送らない):
/// - 同意した版 / 部品ごとのオン・オフ / この端末の senderId(ランダム。アカウントとは無関係)
/// - アプリごとの台帳(受け取った記録の内容・会った人)
@MainActor
@Observable
final class PassbyController: NearbyPassbyController {
    private(set) var consentedVersion: Int?
    private(set) var isAdvertising = false
    private(set) var encounters: [NearbyPassby.Encounter] = []
    private(set) var peers: [PassbyPeer] = []
    private(set) var notice: String?
    var distanceSupport: PassbyDistanceSupport { transport.distanceSupport }

    @ObservationIgnored weak var runtime: AppRuntime?
    @ObservationIgnored private let transport: PassbyTransport
    @ObservationIgnored private let defaults: UserDefaults
    @ObservationIgnored private var current: NearbyPassby.Config?
    @ObservationIgnored private var ledger = NearbyPassby.Ledger()
    @ObservationIgnored private var ledgerAppId: String?
    /// 部品のオン・オフ(観測させるため保存値の写し)
    private var optIns: Set<String> = []

    private enum Key {
        static let consent = "passby.consent"
        static let senderId = "passby.senderId"
        static func optIn(_ appId: String, _ component: String) -> String { "passby.optin.\(appId).\(component)" }
        static func ledger(_ appId: String) -> String { "passby.ledger.\(appId)" }
    }

    init(transport: PassbyTransport, defaults: UserDefaults = .standard) {
        self.transport = transport
        self.defaults = defaults
        consentedVersion = defaults.object(forKey: Key.consent) as? Int
        transport.onPayload = { [weak self] p in Task { await self?.receive(p) } }
        transport.onPeers = { [weak self] in self?.peers = $0 }
        transport.onNotice = { [weak self] in self?.notice = $0 }
    }

    /// 起動時に 1 回。`METAMON_MOCK=1` は通信しないモック。
    static func install(runtime: AppRuntime, mock: Bool) {
        let c = PassbyController(transport: mock ? MockPassbyTransport() : MCPassbyTransport())
        c.runtime = runtime
        NearbyPassbyHost.controller = c
    }

    private var appId: String { runtime?.document.appId ?? "" }

    private var senderId: String {
        if let id = defaults.string(forKey: Key.senderId) { return id }
        let id = UUID().uuidString.lowercased()
        defaults.set(id, forKey: Key.senderId)
        return id
    }

    // MARK: - NearbyPassbyController

    func isOptedIn(_ config: NearbyPassby.Config) -> Bool {
        optIns.contains(Key.optIn(appId, config.componentId)) || defaults.bool(forKey: Key.optIn(appId, config.componentId))
    }

    func setOptIn(_ on: Bool, config: NearbyPassby.Config) {
        let key = Key.optIn(appId, config.componentId)
        defaults.set(on, forKey: key)
        if on { optIns.insert(key) } else { optIns.remove(key) }
        if current?.componentId == config.componentId { restart() }
    }

    func agree() {
        consentedVersion = NearbyPassby.consentVersion
        defaults.set(NearbyPassby.consentVersion, forKey: Key.consent)
    }

    func attach(_ config: NearbyPassby.Config) {
        current = config
        loadLedger()
        if isOptedIn(config) { optIns.insert(Key.optIn(appId, config.componentId)) }
        Task {
            await runtime?.load(entities: [config.entityId]) // 送る記録を読んでおく
            restart()
        }
    }

    func detach(_ config: NearbyPassby.Config) {
        guard current?.componentId == config.componentId else { return }
        current = nil
        stop()
    }

    // MARK: - 開始・停止

    private func restart() {
        guard let config = current, let runtime, let entity = runtime.dsl.entity(config.entityId),
              isOptedIn(config), NearbyPassby.toggle(on: true, consentedVersion: consentedVersion) == .start else {
            stop()
            return
        }
        let me = senderId
        let name = runtime.nearbyDisplayName
        let hello = NearbyPassby.Hello(app: NearbyPassby.appTag(appId), senderId: me, name: name)
        transport.start(PassbyStart(
            hello: hello, config: config, entity: entity,
            payload: { [weak self] in
                guard let self, let rt = self.runtime else {
                    return NearbyPassby.Payload(v: NearbyPassby.version, senderId: me, name: name, entityId: entity.id, records: [])
                }
                return NearbyPassby.makePayload(senderId: me, name: name, entity: entity, config: config,
                                                records: rt.records(for: entity.id), ledger: self.ledger, me: rt.currentUserId)
            },
            shouldExchange: { [weak self] sender in self?.ledger.shouldExchange(with: sender, now: .now) ?? false }))
        isAdvertising = true
    }

    private func stop() {
        transport.stop()
        isAdvertising = false
        peers = []
    }

    // MARK: - 受け取り

    func receive(_ payload: NearbyPassby.Payload) async {
        guard let config = current, let runtime, let entity = runtime.dsl.entity(config.entityId) else { return }
        let items = NearbyPassby.sanitize(payload, into: entity, config: config)
        var changed = 0
        for plan in ledger.plan(items, from: payload.senderId) {
            switch plan {
            case .create(let key, let data):
                if await runtime.createRecord(entity: entity.id, data: data) {
                    ledger.commit(key, data)
                    changed += 1
                }
            case .update(let key, let old, let new):
                // 仮 id は同期で変わるのでローカル DB から読み直して内容で探す。消された記録は作り直さない(内容だけ覚え直す)
                await runtime.reloadCached(entity.id)
                if let rec = NearbyPassby.Ledger.match(old, in: runtime.records(for: entity.id)),
                   await runtime.updateRecord(entity: entity.id, recordId: rec.id, data: new) {
                    changed += 1
                }
                ledger.commit(key, new)
            case .skip:
                break
            }
        }
        ledger.met(payload.senderId, name: payload.name, at: .now, records: changed)
        saveLedger()
        if changed > 0 { runtime.toast = "\(payload.name)さんとすれ違いました(\(entity.name) \(changed) 件)" }
    }

    // MARK: - 台帳

    private func loadLedger() {
        guard ledgerAppId != appId else { return }
        ledgerAppId = appId
        ledger = defaults.data(forKey: Key.ledger(appId)).flatMap { try? NearbyPassby.decode(NearbyPassby.Ledger.self, from: $0) } ?? .init()
        encounters = ledger.encounters
    }

    private func saveLedger() {
        encounters = ledger.encounters
        guard let id = ledgerAppId, let data = try? NearbyPassby.encode(ledger) else { return }
        defaults.set(data, forKey: Key.ledger(id))
    }
}
#endif
