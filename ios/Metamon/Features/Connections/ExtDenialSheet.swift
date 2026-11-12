#if !APP_STORE_REVIEW // 審査用の版は OAuth の provider と App 内課金を外す(決定 6・#450)
import AuthenticationServices
import SwiftUI
import MetamonRenderer
import MetamonServices

/// 外の API の断り(`ExtDenial`・#2001)の「次にすること」。判定は純粋関数(テストする)。
enum ExtDenialRoute: Equatable {
    /// つないでいない → つなぐ案内のシート
    case connect(provider: String)
    /// Pro が要る → 課金画面
    case paywall([PaidFeature])
    /// コイン → `CoinGate`(お店 / コインの設定)。引き受けられなければ説明のトースト
    case coins(CoinDenial)
    /// シートを出さず 1 文だけ
    case toast(String)

    /// アシスティブアクセス(#1997)では課金・コイン・つなぐ案内を出さず、`AssistiveAccessPolicy.unavailableMessage` だけ出す。
    static func of(_ denial: ExtDenial, isPro: Bool, assistiveAccess: AssistiveAccessPolicy = .standard) -> ExtDenialRoute {
        let route = of(denial, isPro: isPro)
        if case .toast = route { return route }
        return assistiveAccess.shows(.paywall) ? route : .toast(AssistiveAccessPolicy.unavailableMessage)
    }

    static func of(_ denial: ExtDenial, isPro: Bool) -> ExtDenialRoute {
        switch denial {
        case .notConnected(let provider?):
            return .connect(provider: provider)
        case .notConnected(nil):
            return .toast(ExtDenialCopy.notConnectedUnknown)
        case .proRequired(let pro):
            if pro.creatorNeedsPro { return .toast(pro.extMessage) } // 参加者は払っても直らない
            if isPro { return .toast(ExtDenialCopy.proNotSynced) } // 端末は Pro なのにサーバーが知らない。課金画面を繰り返さない
            return .paywall(pro.features.isEmpty ? [ConnectedServicesSection.paidFeature] : pro.features)
        case .coins(let coins):
            return .coins(coins)
        case .unavailable:
            return .toast(ExtDenialCopy.unavailable)
        case .busy:
            return .toast(ExtDenialCopy.busy)
        }
    }
}

enum ExtDenialCopy {
    static func connectTitle(_ name: String) -> String { L10n.tr("\(name) とつないでください") }
    static func connectBody(_ name: String) -> String {
        L10n.tr("この部品は \(name) のデータを使います。つなぐと続けられます。つないだサービスは設定からいつでも切れます。")
    }
    static var connectUnavailable: String { L10n.tr("このサービスはいまつなげません。しばらくしてからお試しください。") }
    static var notConnectedUnknown: String { L10n.tr("外のサービスとつながっていません。設定の「つないだサービス」からつないでください。") }
    static var proNotSynced: String { L10n.tr("購入の確認がまだサーバーに届いていません。少し待ってから、もう一度お試しください。") }
    static var unavailable: String { L10n.tr("外のサービスはいま使えません。しばらくしてからお試しください。") }
    static var busy: String { L10n.tr("外のサービスが混み合っています。少し待ってから、もう一度お試しください。") }
    static var closeNote: String { L10n.tr("閉じると、この部品は外のサービスを呼びません。") }
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

    /// `providers` が今つなげないと言っている(古いサーバー・一覧が読めない時はつなげる扱い)
    private var canConnect: Bool { row?.enabled ?? true }

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
                Text(canConnect ? ExtDenialCopy.connectBody(name) : ExtDenialCopy.connectUnavailable)
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
                        if model.busy == provider { ProgressView() } else { Text("つなぐ") }
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
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("閉じる") { dismiss() }
                        .accessibilityIdentifier("ext.connect.close")
                }
            }
        }
        .presentationDetents([.medium])
        .presentationDragIndicator(.visible)
        .task { await model.load(services.external) }
        .onChange(of: row?.status) { _, status in
            if status == .connected { dismiss() } // つなげた
        }
        .sheet(isPresented: $showPaywall) { PaywallSheet(reason: .paidFeature([ConnectedServicesSection.paidFeature])) }
    }
}

#if DEBUG
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
