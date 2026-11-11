// 審査用ビルドは共有リンクの入口を外す(iOS と同じ)。URL スキームの登録(Info.plist)は残るので、届いても何もしない
#if !APP_STORE_REVIEW
import AppKit
import SwiftUI
import MetamonRenderer

/// 外から届いた URL を受ける(#975)。ウインドウの根に 1 行で付ける。
/// - URL スキーム(`tailorit://import/<token>`)は onOpenURL で届く
/// - Universal Links(`https://tailor-it.app/s/<token>`・#1286)は Web 閲覧の NSUserActivity で届く
/// - 連係の Handoff(`HandoffActivity.type`・#1851)は `MacHandoffInbox` へ(アプリを開いてから画面と記録を開く)
///   (entitlement `applinks:$(METAMON_APPLINKS_DOMAIN)`。AASA の appIDs に Mac の App ID が要る・docs/infra/universal-links/README.md)
/// - 起動していない時: macOS がアプリを起動し、最初のウインドウに届く
/// - 開いている時: 新しいウインドウを作らず、今あるウインドウ(手前のもの)で受ける(handlesExternalEvents)
/// - つなぐの戻り先(`tailorit://connected?…`・#1714)は `MacConnectCallbackInbox` に渡し、設定のタブが一覧を取り直す
/// - 知らない URL は何もしない(iOS と同じ)
/// 共有のリンクは `MacShareFlow`(#1287)が iOS と同じ流れで開く。ログインの前に届いたら保留し、ログインできてから 1 回だけ開く。
/// サーバーにつながっていない時(モック・plist 無し)は開けないことを伝え、リンクをコピーできるようにする
struct MacLinkReceiver: ViewModifier {
    @State private var flow = MacShareFlow.shared
    private let session = MacAccount.shared.session

    func body(content: Content) -> some View {
        content
            // OAuth の戻り先(`tailorit://connected?…`・#1714)は設定の「つないだサービス」へ。ほかは共有のリンク
            .onOpenURL { url in
                if !MacConnectCallbackInbox.shared.receive(url) { flow.receive(url) }
            }
            .onContinueUserActivity(NSUserActivityTypeBrowsingWeb) { activity in
                if let url = activity.webpageURL { flow.receive(url) }
            }
            // iPhone・ほかの Mac からの Handoff(#1851): 押された通知と同じ道でアプリを開き、画面は AppCanvas が開く
            .onContinueUserActivity(HandoffActivity.type) { MacHandoffInbox.shared.receive($0) }
            .handlesExternalEvents(preferring: ["*"], allowing: ["*"])
            // 届いた時・ログインできた時に、保留のリンクを開く
            .task(id: "\(session.user?.uid ?? "")|\(flow.link.url?.absoluteString ?? "")") {
                await flow.openPendingIfReady()
            }
            .sheet(item: $flow.pending) { MacShareConfirmSheet(pending: $0, flow: flow) }
            .alert("シェアリンクはサーバー接続時のみ開けます", isPresented: unavailableShown, presenting: flow.unavailable) { url in
                Button("リンクをコピー") { PlatformPasteboard.setString(url.absoluteString) }
                Button("OK", role: .cancel) {}
            } message: { _ in
                Text("共有されたアプリは、今は iPhone の Tailor It で開けます。このリンクを iPhone で開いてください。")
            }
    }

    private var unavailableShown: Binding<Bool> {
        Binding(get: { flow.unavailable != nil }, set: { if !$0 { flow.unavailable = nil } })
    }
}
#endif
