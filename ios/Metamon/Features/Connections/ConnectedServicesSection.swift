#if !APP_STORE_REVIEW // 審査用の版は OAuth の provider を外す(決定 6)
import AuthenticationServices
import SwiftUI
import MetamonServices

/// 設定の「つないだサービス」(#1493): つないだ外のサービスの一覧・つなぐ・つなぎ直す・切る。
/// 一覧はサーバーの `GET /v1/me/connections` から(端末は provider の一覧を持たない)。読み込み中はスケルトン、
/// 読めない時は控えの一覧(`stale`)か再試行。「鍵なしで使えるもの」(OAuth の要らない provider)は別のセクションに名前だけ出す。
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
                section { ConnectedServicesSkeleton() }
            case .failed(let text):
                section {
                    Text(text).foregroundStyle(.secondary)
                    retryButton
                }
            case .loaded(let rows):
                section {
                    if rows.connectable.isEmpty {
                        Text("今つなげるサービスはありません。").foregroundStyle(.secondary)
                    }
                    ForEach(rows.connectable) { row($0) }
                    if model.stale { retryButton }
                }
                if !rows.keyless.isEmpty {
                    Section {
                        ForEach(rows.keyless) { row($0) }
                    } header: {
                        Text("鍵なしで使えるもの")
                    } footer: {
                        Text("サインインしなくても、アプリから使えます。")
                    }
                    .accessibilityIdentifier("settings.connections.keyless")
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

    private var retryButton: some View {
        Button("再試行") { Task { await model.load(services.external) } }
            .accessibilityIdentifier("settings.connections.retry")
    }

    private func section(@ViewBuilder _ content: () -> some View) -> some View {
        Section {
            content()
        } header: {
            Text("つないだサービス")
        } footer: {
            Text(ConnectedServicesCopy.footer(model))
                .foregroundStyle(model.message == nil ? AnyShapeStyle(.secondary) : AnyShapeStyle(Theme.Colors.warning))
        }
        .accessibilityIdentifier("settings.connections")
    }

    private func row(_ row: ConnectedServiceRow) -> some View {
        HStack(spacing: Theme.Spacing.sm) {
            Image(systemName: ConnectedServiceCatalog.symbol(row.provider))
                .foregroundStyle(.secondary)
                .frame(width: 24)
                .accessibilityHidden(true)
            VStack(alignment: .leading, spacing: 2) {
                Text(row.name)
                    .foregroundStyle(row.isUnavailable ? AnyShapeStyle(.secondary) : AnyShapeStyle(.primary))
                Text(ConnectedServicesCopy.status(row))
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
        .disabled(row.isUnavailable || (model.busy != nil && model.busy != row.provider))
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
                if row.enabled {
                    Button("つなぎ直す") { connect(row.provider) }
                        .accessibilityIdentifier("settings.connections.\(row.provider).reconnect")
                }
                Button("切る", role: .destructive) { confirmDisconnect = row }
                    .accessibilityIdentifier("settings.connections.\(row.provider).disconnect")
            }
            .buttonStyle(.borderless)
        case .notConnected: // 今つなげない時は行ごと押せない(row の .disabled)
            Button("つなぐ") { connect(row.provider) }
                .buttonStyle(.borderless)
                .accessibilityIdentifier("settings.connections.\(row.provider).connect")
        case .keyless: // つながなくても使える: 名前と状態だけ
            EmptyView()
        }
    }

    private func connect(_ provider: String) {
        if ConnectFlow.start(provider, model: model, client: services.external, authenticate: ConnectFlow.authenticate(webAuth)) == nil {
            showPaywall = true
        }
    }

    static var paidFeature: PaidFeature { ExtDenialCopy.externalServicesFeature }
}

/// 「つなぐ」の流れ(設定の一覧と、外の API の断りの案内 #2001 で同じものを呼ぶ)。
@MainActor
enum ConnectFlow {
    /// 同意の画面へ進めば、その流れの Task。Pro でなければ何もせず nil(呼び出し側が課金画面を出す)
    @discardableResult
    static func start(_ provider: String, model: ConnectedServicesModel, client: any ExternalServicesClient,
                      isPro: Bool? = nil,
                      authenticate: @escaping ConnectedServicesModel.Authenticate) -> Task<Void, Never>? {
        // 外の API は作成者の Pro を PRO_GATE_MODE に関係なく確かめるので、端末も Pro そのものを見る
        guard isPro ?? ProGate.shared.isPro else { return nil }
        return Task { await model.connect(provider, client: client, authenticate: authenticate) }
    }

    /// 同意の画面(前にサインインしていた人のブラウザのログインを使い回さない)
    static func authenticate(_ webAuth: WebAuthenticationSession) -> ConnectedServicesModel.Authenticate {
        { url in try await webAuth.authenticate(using: url, callbackURLScheme: ConnectCallback.scheme, preferredBrowserSession: .ephemeral) }
    }
}

enum ConnectedServicesCopy {
    /// 今つなげない(つないでいない)行は「いまは使えません」。
    static func status(_ row: ConnectedServiceRow) -> String {
        row.isUnavailable ? L10n.tr("いまは使えません") : status(row.status)
    }

    static func status(_ s: ConnectedServiceStatus) -> String {
        switch s {
        case .connected: L10n.tr("つないでいます")
        case .needsReconnect: L10n.tr("つなぎ直してください")
        case .notConnected: L10n.tr("つないでいません")
        case .keyless: L10n.tr("鍵なしで使えます")
        }
    }

    /// セクションの下の一言: 操作の失敗 → 控えを出している時 → ふだんの説明
    @MainActor static func footer(_ model: ConnectedServicesModel) -> String {
        if let m = model.message { return m }
        if model.stale { return L10n.tr("サーバーに届かないため、前に読み込んだ一覧を出しています。") }
        return L10n.tr("カレンダーやタスクなどの外のサービスとつなぐと、アプリから使えます。Pro で使えます。")
    }
}

/// 読み込み中のスケルトン(データ無しと区別するため必ず出す)。行の形のまま伏せる
struct ConnectedServicesSkeleton: View {
    var rows = 3

    var body: some View {
        ForEach(0..<rows, id: \.self) { _ in
            HStack(spacing: Theme.Spacing.sm) {
                Image(systemName: ConnectedServiceCatalog.fallbackSymbol).frame(width: 24)
                VStack(alignment: .leading, spacing: 2) {
                    Text(verbatim: "Google Calendar")
                    Text(verbatim: "not connected").font(.footnote)
                }
                Spacer()
            }
            .foregroundStyle(.secondary)
            .redacted(reason: .placeholder)
            .accessibilityElement(children: .ignore)
            .accessibilityLabel("読み込み中")
        }
    }
}
#endif
