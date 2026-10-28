// 審査用ビルド(APP_STORE_REVIEW・#245)ではバイナリから外す。
#if !APP_STORE_REVIEW
import SwiftUI
import MetamonDSL
import MetamonRenderer

/// 近距離共有(#36)の present 元: 送信シート(相手選び)と受信の確認ダイアログ。
/// HostSheets と同じく、モーダル表示中は根から present できないので、根とモーダルの両方に付けて「モーダルが出ているか」で片方だけが開く。
struct NearbySheets: ViewModifier {
    let onModal: Bool
    @Environment(AppRuntime.self) private var rt
    @State private var ui = NearbyUI.shared

    func body(content: Content) -> some View {
        let active = onModal == (rt.sheet != nil || rt.fullScreen != nil)
        let incoming = ui.service?.incoming
        content
            .sheet(item: Binding(get: { active ? ui.request : nil }, set: { ui.request = $0 })) { req in
                if let service = ui.service {
                    NearbySendSheet(service: service, payload: req.payload)
                }
            }
            .alert(incomingTitle(incoming), isPresented: Binding(
                get: { active && incoming != nil },
                set: { if !$0, let item = ui.service?.incoming { ui.service?.answer(item, accept: false) } }
            ), presenting: incoming) { item in
                if case .accept = item.decision {
                    Button("受け取る") { ui.service?.answer(item, accept: true) }
                    Button("受け取らない", role: .cancel) { ui.service?.answer(item, accept: false) }
                } else {
                    Button("閉じる", role: .cancel) { ui.service?.answer(item, accept: false) }
                }
            } message: { item in
                switch item.decision {
                case .accept: Text("受け取ると「\(rt.dsl.app.name)」に追加します。")
                case .reject(let reason): Text(reason)
                }
            }
            .modifier(NearbyLifecycle(enabled: !onModal))
    }

    private func incomingTitle(_ item: NearbyService.Incoming?) -> String {
        guard let h = item?.header else { return "" }
        return "\(h.sender)さんから「\(h.entityName)」が \(h.count) 件届きました"
    }
}

/// 根にだけ付ける: 起動時に通信を用意し、前面 & nearbyShare を使うアプリの間だけ待ち受ける。
private struct NearbyLifecycle: ViewModifier {
    let enabled: Bool
    @Environment(AppRuntime.self) private var rt
    @Environment(\.scenePhase) private var scenePhase

    func body(content: Content) -> some View {
        if enabled {
            content
                .task {
                    rt.startNearby()
                    rt.updateNearbyAdvertising(foreground: scenePhase == .active)
                    #if DEBUG
                    await debugHooks()
                    #endif
                }
                .onChange(of: scenePhase) { _, phase in rt.updateNearbyAdvertising(foreground: phase == .active) }
                .onChange(of: rt.dsl.usesNearbyReceive) { _, _ in rt.updateNearbyAdvertising(foreground: scenePhase == .active) }
        } else {
            content
        }
    }

    #if DEBUG
    /// ヘッドレス E2E 用(シミュレータ 2 台):
    /// - 送信側 `METAMON_AUTO_NEARBY_SEND=<entity>` 起動 3 秒後に送信シートを開き、最初に見つかった相手へ送る
    /// - 受信側 `METAMON_AUTO_NEARBY_ACCEPT=1` 招待の確認を 2 秒見せてから受け取る
    private func debugHooks() async {
        let env = ProcessInfo.processInfo.environment
        if env["METAMON_AUTO_NEARBY_ACCEPT"] == "1" {
            Task { @MainActor in
                while !Task.isCancelled {
                    try? await Task.sleep(for: .milliseconds(500))
                    guard let service = NearbyUI.shared.service, let item = service.incoming else { continue }
                    try? await Task.sleep(for: .seconds(2))
                    service.answer(item, accept: true)
                }
            }
        }
        guard let entity = env["METAMON_AUTO_NEARBY_SEND"], !entity.isEmpty else { return }
        try? await Task.sleep(for: .seconds(3))
        await rt.load(entities: [entity])
        rt.shareNearby(entity: entity, records: NearbyShare.recordsToSend(from: rt.records(for: entity)))
        guard let req = NearbyUI.shared.request, let service = NearbyUI.shared.service else { return }
        for _ in 0..<60 {
            try? await Task.sleep(for: .milliseconds(500))
            if let peer = service.peers.first {
                try? await Task.sleep(for: .seconds(1.5))
                service.send(req.payload, to: peer)
                return
            }
        }
        print("[Metamon] nearby: no peer found")
    }
    #endif
}

/// 送信シート: 近くの Tailor It を一覧し、選んだ相手に送る。開いている間だけ探す。
struct NearbySendSheet: View {
    let service: NearbyService
    let payload: NearbyShare.Payload
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        NavigationStack {
            List {
                Section {
                    LabeledContent("送るもの", value: payload.header.song.map { "「\($0.title)」のリクエスト" }
                        ?? "「\(payload.header.entityName)」\(payload.header.count) 件")
                    LabeledContent("あなたの名前", value: service.myName)
                }
                Section {
                    if service.peers.isEmpty {
                        HStack(spacing: 12) {
                            ProgressView()
                            Text("近くの Tailor It を探しています…")
                                .foregroundStyle(.secondary)
                        }
                    }
                    ForEach(service.peers) { peer in
                        Button {
                            service.send(payload, to: peer)
                        } label: {
                            Label(peer.name, systemImage: "iphone.radiowaves.left.and.right")
                        }
                        .disabled(service.sendState.isBusy)
                    }
                } header: {
                    Text("近くの端末")
                } footer: {
                    Text("相手も Tailor It で同じアプリを開いていると表示されます。相手が「受け取る」を押すと届きます。")
                }
                if let status = statusRow {
                    Section { status }
                }
            }
            .navigationTitle("近くの人に送る")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .confirmationAction) {
                    Button("閉じる") { dismiss() }
                }
            }
        }
        .presentationDetents([.medium, .large])
        .onAppear { service.startBrowsing() }
        .onDisappear { service.stopBrowsing() }
        .sensoryFeedback(.success, trigger: service.sendState) { _, new in
            if case .done = new { return true }
            return false
        }
    }

    private var statusRow: AnyView? {
        switch service.sendState {
        case .idle:
            nil
        case .inviting(let name):
            statusLabel("\(name)さんの承認を待っています…", icon: "hourglass", color: .secondary, spinning: true)
        case .sending(let name):
            statusLabel("\(name)さんに送っています…", icon: "arrow.up.circle", color: .secondary, spinning: true)
        case .done(let message):
            statusLabel(message, icon: "checkmark.circle.fill", color: .green, spinning: false)
        case .failed(let message):
            statusLabel(message, icon: "exclamationmark.triangle.fill", color: .orange, spinning: false)
        }
    }

    private func statusLabel(_ text: String, icon: String, color: Color, spinning: Bool) -> AnyView {
        AnyView(HStack(spacing: 12) {
            if spinning { ProgressView() } else { Image(systemName: icon).foregroundStyle(color) }
            Text(text)
        })
    }
}
#endif
