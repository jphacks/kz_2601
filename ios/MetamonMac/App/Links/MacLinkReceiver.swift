// 審査用ビルドは共有リンクの入口を外す(iOS と同じ)。URL スキームの登録(Info.plist)は残るので、届いても何もしない
#if !APP_STORE_REVIEW
import AppKit
import SwiftUI

/// 外から届いた URL を受ける(#975)。ウインドウの根に 1 行で付ける。
/// - URL スキーム(`tailorit://`)は onOpenURL で届く。Mac には associated domains が無いので Universal Links は OS からは来ない。
///   Web 閲覧の NSUserActivity(Handoff など)も読み、中の https が共有ページ(`https://tailor-it.app/s/<token>`・#331)なら同じ案内を出す
/// - 起動していない時: macOS がアプリを起動し、最初のウインドウに届く
/// - 開いている時: 新しいウインドウを作らず、今あるウインドウ(手前のもの)で受ける(handlesExternalEvents)
/// - 知らない URL は何もしない(iOS と同じ)。Mac にはサインインがまだ無いので、保留せずその場で出す
/// 共有のリンクは、Mac にまだサーバーが無い(#930)ので取り込めない。「iPhone で開いてください」と案内する
struct MacLinkReceiver: ViewModifier {
    @State private var received: ReceivedLink?

    func body(content: Content) -> some View {
        content
            .onOpenURL { receive(IncomingLink(url: $0), url: $0) }
            .onContinueUserActivity(NSUserActivityTypeBrowsingWeb) { activity in
                receive(IncomingLink(activity: activity), url: activity.webpageURL)
            }
            .handlesExternalEvents(preferring: ["*"], allowing: ["*"])
            .alert("Mac ではまだ共有のアプリを開けません", isPresented: isPresented, presenting: received) { link in
                Button("リンクをコピー") { Self.copy(link.url) }
                Button("OK", role: .cancel) {}
            } message: { _ in
                Text("共有されたアプリは、今は iPhone の Tailor It で開けます。このリンクを iPhone で開いてください。")
            }
    }

    private func receive(_ link: IncomingLink?, url: URL?) {
        guard let link, let url else { return }
        received = ReceivedLink(link: link, url: url)
    }

    private var isPresented: Binding<Bool> {
        Binding(get: { received != nil }, set: { if !$0 { received = nil } })
    }

    private static func copy(_ url: URL) {
        NSPasteboard.general.clearContents()
        NSPasteboard.general.setString(url.absoluteString, forType: .string)
    }
}

private struct ReceivedLink {
    var link: IncomingLink
    var url: URL
}
#endif
