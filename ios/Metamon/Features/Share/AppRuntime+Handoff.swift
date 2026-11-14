// 審査用ビルドは外から届くもの(シェアリンク)の入口を外すので、Handoff も出さない・受けない(Mac と同じ)
#if !APP_STORE_REVIEW
import SwiftUI
import MetamonServices

/// 連係の Handoff(#1851): 今開いている生成アプリの画面と記録を Mac へ出す・Mac から受ける。決まりは Shared/Runtime/HandoffActivity.swift。
extension AppRuntime {
    /// Handoff に出す今の画面。生成したアプリを開いている時だけ(初期の音声画面・生成中は出さない)
    var handoffTarget: HandoffTarget? {
        guard !dsl.isVoiceOnly, !document.appId.isEmpty, buildStage == nil else { return nil }
        return HandoffTarget(appId: document.appId, navigation: navigationState, dsl: dsl)
    }

    /// 受ける端末にアプリが無い時にブラウザで開く Universal Link。このアプリのシェアリンクを発行済みの時だけ
    static func handoffWebpageURL(_ issued: ShareInfo?) -> URL? {
        guard let issued else { return nil }
        if let web = URL(string: issued.webUrl), web.scheme == "https" { return web }
        return URL(string: issued.url).flatMap { $0.scheme == "https" ? $0 : nil }
    }

    /// 届いた Handoff を開く(`IntentHost` が準備の後に呼ぶ)。別のアプリなら切り替えてから、その画面と記録を開く
    func openHandoff(_ target: HandoffTarget) async {
        if target.appId != document.appId { await switchApp(to: target.appId) }
        guard document.appId == target.appId else { return } // 切替に失敗(トーストは switchApp が出す)
        reveal(target)
    }
}

/// RootView に付ける: 今の画面を Handoff に出す。画面・記録が変わるたびに body が読み直し、SwiftUI が activity を更新する
struct HandoffAdvertiser: ViewModifier {
    @Environment(AppRuntime.self) private var rt

    func body(content: Content) -> some View {
        let target = rt.handoffTarget
        let appName = rt.dsl.app.name
        let url = target.flatMap { AppRuntime.handoffWebpageURL(SharingUI.shared.issued[$0.appId]) }
        content.userActivity(HandoffActivity.type, isActive: target != nil) { activity in
            guard let target else { return }
            MainActor.assumeIsolated {
                HandoffActivity.configure(activity, target: target, appName: appName, webpageURL: url)
            }
        }
    }
}
#endif
