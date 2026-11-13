#if !APP_STORE_REVIEW // 審査用の版は OAuth の provider と App 内課金を外す(決定 6・#450)
import AuthenticationServices
import SwiftUI
import MetamonRenderer
import MetamonServices

extension ExtDenialRoute {
    /// アシスティブアクセス(#1997)では課金・コイン・つなぐ案内(設定・Pro を指す文も)を出さず、`AssistiveAccessPolicy.unavailableMessage` だけ出す。
    static func of(_ denial: ExtDenial, isPro: Bool, assistiveAccess: AssistiveAccessPolicy = .standard) -> ExtDenialRoute {
        guard !assistiveAccess.shows(.paywall) else { return of(denial, isPro: isPro) }
        switch denial {
        case .unavailable, .busy: return of(denial, isPro: isPro) // 待てば直る。行き先を指さない
        default: return .toast(AssistiveAccessPolicy.unavailableMessage)
        }
    }
}

/// 外の API の断りを受けて、シート(つなぐ・課金画面・コイン)かトーストを出す。`denial` に入れると 1 回出して nil に戻す。
/// トーストは呼び出し側の出し方(設定ならセクションの下・アプリなら `rt.toast`)に任せる。
struct ExtDenialPresenter: ViewModifier {
    @Binding var denial: ExtDenial?
    var onMessage: (String) -> Void
    /// つなぐ案内を閉じた時(つないだかもしれないので一覧を取り直す等)
    var onConnectClosed: () -> Void = {}
    @State private var connect: ConnectTarget?
    @State private var paywall: PaywallTarget?
    @Environment(\.assistiveAccess) private var assistiveAccess

    struct ConnectTarget: Identifiable { var provider: String; var id: String { provider } }
    struct PaywallTarget: Identifiable { let id = UUID(); var features: [PaidFeature] }

    func body(content: Content) -> some View {
        content
            .onChange(of: denial) { _, new in
                guard let new else { return }
                denial = nil
                present(new)
            }
            .sheet(item: $connect, onDismiss: onConnectClosed) { ExtConnectSheet(provider: $0.provider) }
            .sheet(item: $paywall) { PaywallSheet(reason: .paidFeature($0.features), closeNote: ExtDenialCopy.closeNote) }
    }

    private func present(_ denial: ExtDenial) {
        print("[Metamon] ext: denied \(denial)")
        switch ExtDenialRoute.of(denial, isPro: ProGate.shared.isPro, assistiveAccess: assistiveAccess) {
        case .connect(let provider): connect = ConnectTarget(provider: provider)
        case .paywall(let features): paywall = PaywallTarget(features: features)
        case .coins(let coins): if !CoinGate.shared.handle(coins) { onMessage(CoinCopy.denialBody(coins)) }
        case .toast(let text): onMessage(text)
        }
    }
}

extension View {
    /// 外の API の断り(#2001)の案内をこの画面の上に出す
    func extDenialSheets(_ denial: Binding<ExtDenial?>, onConnectClosed: @escaping () -> Void = {},
                         onMessage: @escaping (String) -> Void) -> some View {
        modifier(ExtDenialPresenter(denial: denial, onMessage: onMessage, onConnectClosed: onConnectClosed))
    }
}

/// つないでいない(409 `ext_not_connected`)時の案内。「つなぐ」は設定の「つないだサービス」と同じ流れ(`ConnectFlow`)。
/// つなげたら閉じる。
struct ExtConnectSheet: View {
    let provider: String
    @Environment(\.services) private var services
    @Environment(\.webAuthenticationSession) private var webAuth
    @Environment(\.dismiss) private var dismiss
    @State private var model = ConnectedServicesModel()
    @State private var showPaywall = false

    private var row: ConnectedServiceRow? {
        if case .loaded(let rows) = model.state { return rows.first { $0.provider == provider } }
        return nil
    }

    /// サーバーの名前(`providers` の nameJa/nameEn)→ 端末の名前 → id
    private var name: String {
        row?.name ?? ConnectedServicesState.name(nil, id: provider, english: LanguagePreference.shared.code == "en")
    }

    private var availability: ExtConnectAvailability { .of(model.state, provider: provider, providersKnown: model.providersKnown) }
    private var canConnect: Bool { availability == .available }

    var body: some View {
        NavigationStack {
            VStack(alignment: .leading, spacing: Theme.Spacing.lg) {
                HStack(spacing: Theme.Spacing.md) {
                    Image(systemName: ConnectedServiceCatalog.symbol(provider))
                        .font(.title2)
                        .foregroundStyle(.secondary)
                        .accessibilityHidden(true)
                    Text(ExtDenialCopy.connectTitle(name))
                        .font(.system(size: 20, weight: .bold, design: .rounded))
                        .fixedSize(horizontal: false, vertical: true)
                        .accessibilityIdentifier("ext.connect.title")
                }
                Text(availability == .unavailable ? ExtDenialCopy.connectUnavailable : ExtDenialCopy.connectBody(name))
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
                if let message = model.message {
                    Text(message)
                        .font(.footnote)
                        .foregroundStyle(Theme.Colors.warning)
                }
                Button {
                    if ConnectFlow.start(provider, model: model, client: services.external,
                                         authenticate: ConnectFlow.authenticate(webAuth)) == nil {
                        showPaywall = true
                    }
                } label: {
                    Group {
                        if model.busy == provider || availability == .loading { ProgressView() } else { Text("つなぐ") }
                    }
                    .frame(maxWidth: .infinity)
                }
                .buttonStyle(.borderedProminent)
                .controlSize(.large)
                .disabled(!canConnect || model.busy != nil)
                .accessibilityIdentifier("ext.connect.button")
                Spacer(minLength: 0)
            }
            .padding(Theme.Spacing.lg)
            .background(Theme.Colors.canvas.ignoresSafeArea())
            .toolbar { SheetCloseToolbarItem(identifier: "ext.connect.close") { dismiss() } }
        }
        .presentationDetents([.medium])
        .presentationDragIndicator(.visible)
        .task { await model.load(services.external) }
        .onChange(of: model.state) { old, new in
            if ExtConnectAvailability.didConnect(from: old, to: new, provider: provider) { dismiss() } // つなげた
        }
        .sheet(isPresented: $showPaywall) { PaywallSheet(reason: .paidFeature([ConnectedServicesSection.paidFeature])) }
    }
}

#if DEBUG || METAMON_STG // 開発用メニュー(#2079)から呼ぶ
/// 開発用: 偽の provider `fakeOauth` に op を投げて、断りの案内を部品なしで出す(#2001)。
/// `METAMON_MOCK=1` と `METAMON_MOCK_EXT_ERROR=<code>` を併せると、モックの次の呼び出しにそのエラーを仕込む(UI テスト用)。
enum DebugExtDenial {
    static let provider = "fakeOauth"
    static let op = "listItems"

    /// 仕込むエラー(コードは openapi の tag `ext`・runner と同じ)
    static func error(code: String?) -> APIError? {
        switch code {
        case "ext_not_connected": .server(status: 409, code: "ext_not_connected", message: "", details: ["provider": .string(provider)])
        case "pro_required": .server(status: 402, code: "pro_required", message: "", details: [:])
        case "coins_insufficient": .server(status: 402, code: "coins_insufficient", message: "", details: ["required": .number(5), "balance": .number(1)])
        case "ext_unavailable": .server(status: 503, code: "ext_unavailable", message: "", details: [:])
        case "ext_busy": .server(status: 503, code: "ext_busy", message: "", details: [:])
        default: nil
        }
    }

    /// 呼んで、断られたら `ExtDenial`。成功・ほかの失敗は nil と文言
    static func call(_ client: any ExternalServicesClient) async -> (ExtDenial?, String?) {
        if let mock = client as? MockExternalServicesClient,
           let e = error(code: ProcessInfo.processInfo.environment["METAMON_MOCK_EXT_ERROR"]) {
            await mock.setNextError(e)
        }
        do {
            _ = try await client.call(appId: "debug", provider: provider, op: op, args: [:], requestId: UUID().uuidString)
            return (nil, "呼べました(開発用)")
        } catch {
            if let denial = ExtDenial(error) { return (denial, nil) }
            return (nil, "呼べませんでした(開発用): \(error.localizedDescription)")
        }
    }
}
#endif
#endif
