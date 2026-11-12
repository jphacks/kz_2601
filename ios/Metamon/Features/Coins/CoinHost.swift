#if !APP_STORE_REVIEW // 審査用ビルドは App 内課金を持たない(#450)
import SwiftUI
import MetamonServices

/// コインのシート(お店・プランを比べる・コインの設定)を出す。見えている presenter のうち、いちばん手前のものが出す
/// (シートの上には別のシートを重ねられないので、設定・課金画面・音声シートにもそれぞれ付ける)。
struct CoinSheetPresenter: ViewModifier {
    @State private var id = UUID()
    @Environment(\.assistiveAccess) private var assistiveAccess
    @Environment(AppRuntime.self) private var rt: AppRuntime?

    func body(content: Content) -> some View {
        let gate = CoinGate.shared
        let shows = assistiveAccess.shows(.paywall) // アシスティブアクセス(#1997)ではお店・コインの設定を出さない
        return content
            .onAppear { gate.presenterAppeared(id) }
            .onDisappear { gate.presenterDisappeared(id) }
            .sheet(item: Binding(
                get: { gate.request(for: id, assistiveAccess: assistiveAccess) },
                set: { if $0 == nil, gate.request?.presenter == id { gate.dismiss() } }
            )) { request in
                switch request.sheet {
                case .shop(let reason): CoinShopSheet(reason: reason)
                case .plans: PlanComparisonSheet()
                case .settings(let reason): CoinSettingsSheet(reason: reason)
                }
            }
            // 出さない時はすぐ閉じて一言だけ
            .onChange(of: gate.request?.id, initial: true) {
                guard !shows, gate.request?.presenter == id else { return }
                gate.dismiss()
                rt?.toast = AssistiveAccessPolicy.unavailableMessage
            }
    }
}

/// 起動時にコインの口を入れて設定(フラグ)を読む + いちばん奥の presenter。`PaywallHost` に付ける。
struct CoinHost: ViewModifier {
    @Environment(\.services) private var services

    func body(content: Content) -> some View {
        content
            .modifier(CoinSheetPresenter())
            .task {
                CoinGate.shared.install(service: services.coins) { [auth = services.auth] in auth.currentUser?.uid }
            }
    }
}

extension View {
    /// このシートの上にコインのシートを出せるようにする
    func coinSheets() -> some View { modifier(CoinSheetPresenter()) }
}
#endif
