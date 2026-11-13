#if DEBUG && METAMON_STG
import SwiftUI

/// 開発用メニュー: 通知のボタンと返信(#2231)を試す。DEBUG の版だけ。
final class DebugNotificationActionsSection: DebugMenuSection {
    override class var order: Int { 280 }
    override class var title: String { "通知のボタンと返信" }
    override class var footer: String? {
        "5 秒後にボタン付きの通知を出します。ホームに戻って待ち、通知を長押しして「記録する」か「返信」を押すと、アプリに戻った時にトーストで受け取った内容が出ます(打った文字そのものは出しません)。"
    }
    @MainActor override class func makeBody() -> AnyView { AnyView(DebugNotificationActionsRows()) }
}

private struct DebugNotificationActionsRows: View {
    @Environment(AppRuntime.self) private var rt
    @State private var status: String?

    var body: some View {
        Button {
            Task {
                do {
                    try await NotificationCategories.postTest(appId: rt.document.appId)
                    status = "5 秒後に届きます"
                } catch {
                    status = "出せませんでした(通知の許可を確かめてください)"
                }
            }
        } label: {
            DebugKnobLabel(title: "試験用の通知を出す", effect: .immediate)
        }
        .accessibilityIdentifier("debugMenu.notificationActions.post")
        if let status { Text(verbatim: status).font(.footnote) }
    }
}
#endif
