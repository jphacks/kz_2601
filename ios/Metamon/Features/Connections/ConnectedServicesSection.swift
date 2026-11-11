#if !APP_STORE_REVIEW // 審査用の版は OAuth の provider を外す(決定 6)
import AuthenticationServices
import SwiftUI
import MetamonServices

/// 設定の「つないだサービス」(#1493): つないだ外のサービスの一覧・つなぐ・つなぎ直す・切る。
/// Pro だけ(Pro でなければ課金画面)。サーバーが外の API を止めている(`ext_unavailable`)時はセクションごと出さない。
struct ConnectedServicesSection: View {
    @Environment(\.services) private var services
    @Environment(\.webAuthenticationSession) private var webAuth
    @State private var model = ConnectedServicesModel()
    @State private var confirmDisconnect: ConnectedServiceRow?
    @State private var showPaywall = false

    var body: some View {
        Group {
            switch model.state {
            case .hidden:
                EmptyView()
            case .loading:
                section { HStack { ProgressView(); Text("読み込み中").foregroundStyle(.secondary) } }
            case .failed(let text):
                section {
                    Text(text).foregroundStyle(.secondary)
                    Button("再試行") { Task { await model.load(services.external) } }
                        .accessibilityIdentifier("settings.connections.retry")
                }
            case .loaded(let rows):
                section {
                    ForEach(rows) { row($0) }
                }
            }
        }
        .task { await model.load(services.external) }
        .sheet(isPresented: $showPaywall) { PaywallSheet(reason: .paidFeature([Self.paidFeature])) }
        .confirmationDialog(confirmTitle, isPresented: Binding(get: { confirmDisconnect != nil }, set: { if !$0 { confirmDisconnect = nil } }),
                            titleVisibility: .visible, presenting: confirmDisconnect) { row in
            Button("切る", role: .destructive) {
                Task { await model.disconnect(row.provider, client: services.external) }
            }
            .accessibilityIdentifier("settings.connections.disconnect.confirm")
            Button("キャンセル", role: .cancel) {}
        } message: { _ in
            Text("切ると、このサービスを使うアプリの部品が動かなくなります。もう一度つなげば戻ります。")
        }
    }

    private var confirmTitle: String {
        L10n.tr("\(confirmDisconnect?.name ?? "") とのつなぎを切りますか？")
    }

    private func section(@ViewBuilder _ content: () -> some View) -> some View {
        Section {
            content()
        } header: {
            Text("つないだサービス")
        } footer: {
            Text(model.message ?? L10n.tr("カレンダーやタスクなどの外のサービスとつなぐと、アプリから使えます。Pro で使えます。"))
                .foregroundStyle(model.message == nil ? AnyShapeStyle(.secondary) : AnyShapeStyle(Theme.Colors.warning))
        }
        .accessibilityIdentifier("settings.connections")
    }

    private func row(_ row: ConnectedServiceRow) -> some View {
        HStack(spacing: Theme.Spacing.sm) {
            VStack(alignment: .leading, spacing: 2) {
                Text(row.name)
                Text(ConnectedServicesCopy.status(row.status))
                    .font(.footnote)
                    .foregroundStyle(row.status == .needsReconnect ? AnyShapeStyle(Theme.Colors.warning) : AnyShapeStyle(.secondary))
            }
            Spacer()
            if model.busy == row.provider {
                ProgressView()
            } else {
                actions(row)
            }
        }
        .disabled(model.busy != nil && model.busy != row.provider)
        .accessibilityIdentifier("settings.connections.\(row.provider)")
    }

    @ViewBuilder
    private func actions(_ row: ConnectedServiceRow) -> some View {
        switch row.status {
        case .connected:
            Button("切る", role: .destructive) { confirmDisconnect = row }
                .buttonStyle(.borderless)
                .accessibilityIdentifier("settings.connections.\(row.provider).disconnect")
        case .needsReconnect:
            HStack(spacing: Theme.Spacing.md) {
                Button("つなぎ直す") { connect(row.provider) }
                    .accessibilityIdentifier("settings.connections.\(row.provider).reconnect")
                Button("切る", role: .destructive) { confirmDisconnect = row }
                    .accessibilityIdentifier("settings.connections.\(row.provider).disconnect")
            }
            .buttonStyle(.borderless)
        case .notConnected:
            Button("つなぐ") { connect(row.provider) }
                .buttonStyle(.borderless)
                .accessibilityIdentifier("settings.connections.\(row.provider).connect")
        }
    }

    private func connect(_ provider: String) {
        // 外の API は作成者の Pro を PRO_GATE_MODE に関係なく確かめるので、端末も Pro そのものを見る
        guard ProGate.shared.isPro else {
            showPaywall = true
            return
        }
        let webAuth = webAuth
        Task {
            await model.connect(provider, client: services.external) { url in
                try await webAuth.authenticate(using: url, callbackURLScheme: ConnectCallback.scheme)
            }
        }
    }

    static var paidFeature: PaidFeature {
        PaidFeature(id: "externalServices", title: L10n.tr("つないだサービス"),
                    reason: L10n.tr("外のサービスを呼ぶたびにサーバーの費用がかかるため、Pro の機能です。"))
    }
}

enum ConnectedServicesCopy {
    static func status(_ s: ConnectedServiceStatus) -> String {
        switch s {
        case .connected: L10n.tr("つないでいます")
        case .needsReconnect: L10n.tr("つなぎ直してください")
        case .notConnected: L10n.tr("つないでいません")
        }
    }
}
#endif
