#if !APP_STORE_REVIEW // 審査用の版は OAuth の provider と App 内課金を外す(決定 6・#450)
import SwiftUI
import MetamonRenderer
import MetamonServices

/// 外の API の断り(`ExtDenial`・#2001)の「次にすること」を Mac のシートで出す(#3078)。iOS の `ExtDenialPresenter` の Mac 版。
/// 判定と文言は iOS と共有(`ExtDenialRoute` / `ExtDenialCopy`)。`denial` に入れると 1 回出して nil に戻す。
/// - つないでいない → つなぐ案内のシート(つなぐ先は設定の「つないだサービス」と同じ)
/// - Pro が要る → Mac の課金画面
/// - コイン → Mac にコインの画面は無いので、断りの説明だけ `onMessage` で出す
struct MacExtDenialPresenter: ViewModifier {
    @Binding var denial: ExtDenial?
    var onMessage: (String) -> Void
    /// つなぐ案内を閉じた時(つないだかもしれないので一覧を取り直す等)
    var onConnectClosed: () -> Void = {}
    @State private var connect: ConnectTarget?
    @State private var paywall: PaywallTarget?

    struct ConnectTarget: Identifiable { var provider: String; var id: String { provider } }
    struct PaywallTarget: Identifiable { let id = UUID(); var features: [PaidFeature] }

    func body(content: Content) -> some View {
        content
            .onChange(of: denial) { _, new in
                guard let new else { return }
                denial = nil
                present(new)
            }
            .sheet(item: $connect, onDismiss: onConnectClosed) { MacExtConnectSheet(provider: $0.provider) }
            .sheet(item: $paywall) { MacPaywallView(reason: .paidFeature($0.features), closeNote: ExtDenialCopy.closeNote) }
    }

    private func present(_ denial: ExtDenial) {
        switch ExtDenialRoute.of(denial, isPro: MacPro.shared.isPro) {
        case .connect(let provider): connect = ConnectTarget(provider: provider)
        case .paywall(let features): paywall = PaywallTarget(features: features)
        case .coins(let coins): onMessage(CoinCopy.denialBody(coins))
        case .toast(let text): onMessage(text)
        }
    }
}

extension View {
    /// 外の API の断り(#2001)の案内をこの画面の上に出す(Mac・#3078)
    func macExtDenialSheets(_ denial: Binding<ExtDenial?>, onConnectClosed: @escaping () -> Void = {},
                            onMessage: @escaping (String) -> Void) -> some View {
        modifier(MacExtDenialPresenter(denial: denial, onMessage: onMessage, onConnectClosed: onConnectClosed))
    }
}

/// つないでいない(409 `ext_not_connected`)時の案内。「つなぐ」は設定の「つないだサービス」と同じ流れ。つなげたら閉じる。
struct MacExtConnectSheet: View {
    let provider: String
    var client: (any ExternalServicesClient)? = MacServices.shared.external
    @Environment(\.dismiss) private var dismiss
    @State private var model = ConnectedServicesModel()
    @State private var showPaywall = false

    private var row: ConnectedServiceRow? {
        if case .loaded(let rows) = model.state { return rows.first { $0.provider == provider } }
        return nil
    }

    private var name: String {
        row?.name ?? ConnectedServicesState.name(nil, id: provider, english: LanguagePreference.shared.code == "en")
    }

    private var availability: ExtConnectAvailability { .of(model.state, provider: provider, providersKnown: model.providersKnown) }
    private var canConnect: Bool { client != nil && availability == .available }

    var body: some View {
        VStack(alignment: .leading, spacing: Theme.Spacing.lg) {
            Text(ExtDenialCopy.connectTitle(name))
                .font(.title3.weight(.bold))
                .fixedSize(horizontal: false, vertical: true)
                .accessibilityIdentifier("ext.connect.title")
            Text(canConnect || availability == .loading ? ExtDenialCopy.connectBody(name) : ExtDenialCopy.connectUnavailable)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            if let message = model.message {
                Text(message).font(.footnote).foregroundStyle(Theme.Colors.warning)
            }
            HStack {
                Spacer()
                Button("閉じる") { dismiss() }
                    .keyboardShortcut(.cancelAction)
                    .accessibilityIdentifier("ext.connect.close")
                Button {
                    connect()
                } label: {
                    if model.busy == provider || availability == .loading { ProgressView().controlSize(.small) } else { Text("つなぐ") }
                }
                .keyboardShortcut(.defaultAction)
                .disabled(!canConnect || model.busy != nil)
                .accessibilityIdentifier("ext.connect.button")
            }
        }
        .padding(Theme.Spacing.lg)
        .frame(width: 420)
        .task { if let client { await model.load(client) } }
        .onChange(of: model.state) { old, new in
            if ExtConnectAvailability.didConnect(from: old, to: new, provider: provider) { dismiss() } // つなげた
        }
        .sheet(isPresented: $showPaywall) { MacPaywallView(reason: .paidFeature([ExtDenialCopy.externalServicesFeature])) }
    }

    private func connect() {
        guard let client else { return }
        // 外の API は作成者の Pro を確かめるので、設定と同じく Pro そのものを見る
        guard MacPro.shared.isPro else {
            showPaywall = true
            return
        }
        Task {
            await model.connect(provider, client: client) { url in
                try await MacWebAuthenticator().authenticate(url: url, callbackScheme: ConnectCallback.scheme)
            }
        }
    }
}
#endif
