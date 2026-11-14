// 審査用ビルド(APP_STORE_REVIEW・#245)ではバイナリから外す(iOS と同じ)。
#if !APP_STORE_REVIEW
import SwiftUI
import AppKit
import Observation
import MetamonDSL
import MetamonRenderer

/// 近くの人に送る(`nearbyShare`・#964)の Mac 側の状態。通信は iOS と同じ `NearbyService`(MultipeerConnectivity)なので、
/// iPhone・iPad・Mac の Tailor It と送り合える。すれ違い通信(passby)は Mac では使えない(Renderer が「この Mac では使えません」の枠を出す)。
///
/// 通信はアプリに 1 つ。ウインドウごとにランタイム(`MacAppRuntime`)があるので、受け取りの判定と取り込みは
/// いちばん最近前面にしたウインドウのランタイム(`active`)で行い、確認もそのウインドウにだけ出す。
@MainActor
@Observable
final class MacNearby {
    static let shared = MacNearby()

    struct SendRequest: Identifiable {
        let id = UUID()
        let payload: NearbyShare.Payload
        /// 送るボタンを押したウインドウのランタイム。シートはそのウインドウにだけ出す
        let owner: ObjectIdentifier
    }

    /// 送信シート(相手選び)。nil でなければ `MacNearbySheets` が出す
    var request: SendRequest?
    /// 通信本体(最初に要った時に作る・作るだけでは通信しない)
    private(set) var service: NearbyService?
    /// 受け取りの確認を出すウインドウ
    private(set) var activeId: ObjectIdentifier?
    @ObservationIgnored private(set) weak var active: MacAppRuntime?
    @ObservationIgnored private var appIsActive = true
    @ObservationIgnored private var observers: [NSObjectProtocol] = []

    /// 「名前を出さない」。iOS のすれ違い通信と同じキー(`PassbyController`)
    static let anonymousKey = "passby.anonymous"
    @ObservationIgnored private let defaults: UserDefaults
    var isAnonymous: Bool {
        didSet {
            defaults.set(isAnonymous, forKey: Self.anonymousKey)
            service?.rename(publicName)
        }
    }

    /// サインインしている名前(無ければ Mac の名前)。テストで差し替える
    @ObservationIgnored var signedInName: @MainActor () -> String? = { MacServices.shared.auth.currentUser?.displayName }

    init(defaults: UserDefaults = .standard) {
        self.defaults = defaults
        isAnonymous = defaults.bool(forKey: Self.anonymousKey)
        let center = NotificationCenter.default
        observers = [
            center.addObserver(forName: NSApplication.didBecomeActiveNotification, object: nil, queue: .main) { [weak self] _ in
                MainActor.assumeIsolated { self?.setAppActive(true) }
            },
            center.addObserver(forName: NSApplication.didResignActiveNotification, object: nil, queue: .main) { [weak self] _ in
                MainActor.assumeIsolated { self?.setAppActive(false) }
            },
        ]
    }

    /// 自分の表示名(相手の一覧と「◯◯さんから」に出る)。サインイン名 → Mac の名前。
    var displayName: String {
        if let name = signedInName(), !name.isEmpty { return name }
        return Host.current().localizedName ?? "Mac"
    }

    /// 相手に見える名前(MCPeerID と Header の送り主)
    var publicName: String { NearbyLimits.peerName(displayName, anonymous: isAnonymous) }

    /// 通信を用意する(受け取りの判定と取り込みは `active` のランタイムへ)。
    @discardableResult
    func start() -> NearbyService {
        if let service { return service }
        let created = NearbyService(displayName: publicName)
        created.decide = { [weak self] header in
            guard let rt = self?.active else { return .reject(reason: L10n.tr("今は受け取れません")) }
            return NearbyShare.decide(header, dsl: rt.dsl, appId: rt.appId)
        }
        created.onReceive = { [weak self] payload, _ in
            guard let rt = self?.active else { return .init(ok: false, added: 0, message: L10n.tr("今は受け取れません")) }
            return await rt.importNearby(payload)
        }
        service = created
        return created
    }

    // MARK: - 待ち受け

    /// 前面にいて、前面のウインドウのアプリが `nearbyShare`(送る方)か曲のリクエストを使っている間だけ待ち受ける。
    /// (使わないアプリでは待ち受けない = ローカルネットワークの許可を不意に求めない・iOS と同じ)
    static func shouldAdvertise(appIsActive: Bool, dsl: AppDSL?) -> Bool {
        appIsActive && dsl?.usesNearbyReceive == true
    }

    /// このウインドウを受け取りの相手にする(前面になった・開いた)
    func activate(_ runtime: MacAppRuntime) {
        active = runtime
        activeId = ObjectIdentifier(runtime)
        updateAdvertising()
    }

    /// ウインドウを閉じた・アプリを切り替えた
    func deactivate(_ runtime: MacAppRuntime) {
        guard activeId == ObjectIdentifier(runtime) else { return }
        active = nil
        activeId = nil
        if request?.owner == ObjectIdentifier(runtime) { request = nil }
        updateAdvertising()
    }

    private func setAppActive(_ on: Bool) {
        appIsActive = on
        updateAdvertising()
    }

    private func updateAdvertising() {
        if Self.shouldAdvertise(appIsActive: appIsActive, dsl: active?.dsl) {
            start().setAdvertising(true)
        } else {
            service?.setAdvertising(false)
        }
    }
}

extension MacAppRuntime: NearbyHost {
    /// `nearbyShare` から: 送るデータを作って相手選びのシートを開く(iOS の `AppRuntime.shareNearby` と同じ)。
    func shareNearby(entity: String, records: [[String: JSONValue]]) {
        guard let e = dsl.entity(entity) else { return }
        let nearby = MacNearby.shared
        let payload = NearbyShare.makePayload(sender: nearby.service?.myName ?? nearby.publicName, appId: appId,
                                              appName: dsl.app.name, entity: e, records: records)
        guard payload.header.count > 0 else {
            toast = L10n.tr("送る記録がありません")
            return
        }
        nearby.request = .init(payload: payload, owner: ObjectIdentifier(self))
    }

    /// `musicPlayer` から(#412): 曲のリクエストを作って相手選びのシートを開く。
    func requestSong(_ song: MusicSong) {
        let nearby = MacNearby.shared
        guard let payload = NearbyShare.makeSongRequest(song, sender: nearby.service?.myName ?? nearby.publicName,
                                                        appId: appId, appName: dsl.app.name) else {
            toast = L10n.tr("曲名を入れてください")
            return
        }
        nearby.request = .init(payload: payload, owner: ObjectIdentifier(self))
    }

    /// 受け取った記録を今のアプリに足す。招待の後にアプリが切り替わることもあるので、ここでも判定し直す(iOS と同じ)。
    func importNearby(_ payload: NearbyShare.Payload) async -> NearbyShare.Ack {
        let h = payload.header
        if let song = h.song { return await importSongRequest(song, from: NearbyLimits.displayName(h.sender)) }
        switch NearbyShare.decide(h, dsl: dsl, appId: appId) {
        case .reject(let reason):
            toast = reason
            return .init(ok: false, added: 0, message: reason)
        case .accept(let entityId, let sameApp):
            guard let entity = dsl.entity(entityId) else { return .init(ok: false, added: 0) }
            let rows = NearbyShare.sanitize(payload.records, into: entity, sameApp: sameApp)
            guard !rows.isEmpty else {
                toast = L10n.tr("\(NearbyLimits.displayName(h.sender))さんから届いた記録に、追加できる項目がありませんでした")
                return .init(ok: false, added: 0, message: L10n.tr("追加できる項目がありませんでした"))
            }
            var added = 0
            for row in rows {
                if await createRecordId(entity: entityId, data: row) != nil { added += 1 }
            }
            await reloadLocal(entityId)
            guard added > 0 else {
                toast = RecordActionSteps.saveFailedMessage
                return .init(ok: false, added: 0, message: RecordActionSteps.saveFailedMessage)
            }
            toast = L10n.tr("\(NearbyLimits.displayName(h.sender))さんから「\(entity.name)」を \(added) 件追加しました")
            return .init(ok: true, added: added)
        }
    }

    /// 曲のリクエストを musicPlayer の承認待ちへ(autoAccept なら再生待ちへ)。
    func importSongRequest(_ song: MusicSong, from sender: String) async -> NearbyShare.Ack {
        guard let player = dsl.musicPlayer, player.allowRequests == true else {
            return .init(ok: false, added: 0, message: L10n.tr("相手のアプリは今、曲のリクエストを受け付けていません"))
        }
        return await MusicSession.shared.receive(song, from: sender, autoAccept: player.autoAccept == true)
    }
}

/// 根のウインドウにだけ付ける: 開いた・前面になったウインドウを受け取りの相手にし、閉じたら外す。
struct MacNearbyLifecycle: ViewModifier {
    let runtime: MacAppRuntime
    @Environment(\.controlActiveState) private var controlActiveState

    func body(content: Content) -> some View {
        content
            .onAppear { MacNearby.shared.activate(runtime) }
            .onDisappear { MacNearby.shared.deactivate(runtime) }
            .onChange(of: controlActiveState) { _, state in
                if state == .key { MacNearby.shared.activate(runtime) }
            }
    }
}

/// 送信シートと受け取りの確認。`active` は確認ダイアログと同じ(シートが開いている間はシートの中だけに出す)。
struct MacNearbySheets: ViewModifier {
    let runtime: MacAppRuntime
    let active: Bool
    @State private var nearby = MacNearby.shared

    private var mine: Bool { nearby.activeId == ObjectIdentifier(runtime) }

    func body(content: Content) -> some View {
        let incoming = nearby.service?.incoming
        content
            .sheet(item: Binding(
                get: { active && nearby.request?.owner == ObjectIdentifier(runtime) ? nearby.request : nil },
                set: { nearby.request = $0 }
            )) { req in
                MacNearbySendSheet(service: nearby.start(), payload: req.payload, nearby: nearby)
            }
            .alert(incomingTitle(incoming), isPresented: Binding(
                get: { active && mine && incoming != nil },
                set: { if !$0, let item = nearby.service?.incoming { nearby.service?.answer(item, accept: false) } }
            ), presenting: incoming) { item in
                if case .accept = item.decision {
                    Button("受け取る") { nearby.service?.answer(item, accept: true) }
                    Button("受け取らない", role: .cancel) { nearby.service?.answer(item, accept: false) }
                } else {
                    Button("閉じる", role: .cancel) { nearby.service?.answer(item, accept: false) }
                }
            } message: { item in
                switch item.decision {
                case .accept: Text("受け取ると「\(runtime.dsl.app.name)」に追加します。")
                case .reject(let reason): Text(reason)
                }
            }
    }

    private func incomingTitle(_ item: NearbyService.Incoming?) -> String {
        guard let h = item?.header else { return "" }
        return L10n.tr("\(NearbyLimits.displayName(h.sender))さんから「\(h.entityName)」が \(h.count) 件届きました")
    }
}

/// 送信シート: 近くの Tailor It を一覧し、選んだ相手に送る。開いている間だけ探す(iOS の `NearbySendSheet` の Mac 版)。
struct MacNearbySendSheet: View {
    let service: NearbyService
    let payload: NearbyShare.Payload
    @Bindable var nearby: MacNearby
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        VStack(spacing: 0) {
            Text("近くの人に送る")
                .font(.headline)
                .padding(.top, 16)
            Form {
                Section {
                    LabeledContent("送るもの", value: payload.header.song.map { L10n.tr("「\($0.title)」のリクエスト") }
                        ?? L10n.tr("「\(payload.header.entityName)」\(payload.header.count) 件"))
                    Toggle("名前を出さない", isOn: $nearby.isAnonymous)
                        .disabled(service.sendState.isBusy)
                        .accessibilityIdentifier("nearby.send.anonymous")
                    LabeledContent("相手に見える名前", value: NearbyLimits.displayName(service.myName))
                        .accessibilityIdentifier("nearby.send.publicName")
                } footer: {
                    Text("オンにすると、名前の代わりに「近くの人」と表示されます。")
                }
                Section {
                    if service.peers.isEmpty {
                        HStack(spacing: 12) {
                            ProgressView().controlSize(.small)
                            Text("近くの Tailor It を探しています…")
                                .foregroundStyle(.secondary)
                        }
                    }
                    ForEach(service.peers) { peer in
                        Button {
                            service.send(payload, to: peer)
                        } label: {
                            Label(peer.name, systemImage: "antenna.radiowaves.left.and.right")
                        }
                        .disabled(service.sendState.isBusy)
                    }
                } header: {
                    Text("近くの端末")
                } footer: {
                    VStack(alignment: .leading, spacing: 6) {
                        Text("相手も Tailor It で同じアプリを開いていると表示されます。相手が「受け取る」を押すと届きます。")
                        // 同じ仕組み(MultipeerConnectivity)の端末どうしだけつながる。Android は Nearby Connections
                        Text("iPhone・iPad・Mac の Tailor It と送り合えます。Android の端末とはつながりません")
                            .accessibilityIdentifier("nearby.send.platformNote")
                    }
                }
                if let status = statusRow {
                    Section { status }
                }
            }
            .formStyle(.grouped)
            HStack {
                Spacer()
                Button("閉じる") { dismiss() }
                    .keyboardShortcut(.cancelAction)
            }
            .padding(16)
        }
        .frame(minWidth: 440, minHeight: 440)
        .onAppear { service.startBrowsing() }
        .onDisappear { service.stopBrowsing() }
    }

    private var statusRow: AnyView? {
        switch service.sendState {
        case .idle:
            nil
        case .inviting(let name):
            statusLabel(L10n.tr("\(name)さんの承認を待っています…"), icon: "hourglass", color: .secondary, spinning: true)
        case .sending(let name):
            statusLabel(L10n.tr("\(name)さんに送っています…"), icon: "arrow.up.circle", color: .secondary, spinning: true)
        case .done(let message):
            statusLabel(message, icon: "checkmark.circle.fill", color: .green, spinning: false)
        case .failed(let message):
            statusLabel(message, icon: "exclamationmark.triangle.fill", color: .orange, spinning: false)
        }
    }

    private func statusLabel(_ text: String, icon: String, color: Color, spinning: Bool) -> AnyView {
        AnyView(HStack(spacing: 12) {
            if spinning { ProgressView().controlSize(.small) } else { Image(systemName: icon).foregroundStyle(color) }
            Text(text)
        })
    }
}
#endif
