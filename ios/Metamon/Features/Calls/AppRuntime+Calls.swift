// 審査用ビルド(APP_STORE_REVIEW)は通話(#447)を外す。
#if !APP_STORE_REVIEW
import SwiftUI
import MetamonDSL
import MetamonRenderer
import MetamonServices

/// 通話の画面に出している通話(nil = 出していない)。AppRuntime(@Observable)には拡張で保存プロパティを足せないので別に持つ。
@MainActor
@Observable
final class CallUI {
    static let shared = CallUI()

    var active: ActiveCall?
    /// コインが足りない時に購入画面を開く(#630 のコインの画面が入ったらつなぐ)。nil = 案内の文だけ出す。
    @ObservationIgnored var openCoinShop: ((_ required: Int, _ balance: Int) -> Void)?
}

/// 通話の画面 1 回ぶん。
struct ActiveCall: Identifiable {
    let id = UUID()
    let title: String
    let session: CallSession
}

extension AppRuntime: CallHost {
    /// 共有のアプリで、live で、サインインしている時だけ。
    var canCall: Bool {
        services.calls != nil && !(currentUserId ?? "").isEmpty && dsl.entities.contains(where: \.isShared)
    }

    func startCall(room: String?, video: Bool, title: String) {
        guard let client = services.calls, canCall, CallUI.shared.active == nil else { return }
        let session = CallSession(appId: document.appId, roomKey: room, video: video, name: AuthorProfile.name,
                                  client: client, room: LiveKitCallRoom())
        CallUI.shared.active = ActiveCall(title: title, session: session)
    }
}

/// 通話の画面。`onModal` は MatchingSheets と同じ(DSL のシートの上か下か)。
struct CallSheets: ViewModifier {
    var onModal = false
    @Environment(AppRuntime.self) private var rt
    @State private var ui = CallUI.shared

    func body(content: Content) -> some View {
        content.fullScreenCover(item: Binding(
            get: { onModal == (rt.sheet != nil || rt.fullScreen != nil) ? ui.active : nil },
            set: { if $0 == nil, ui.active != nil { ui.active = nil } }
        )) { call in
            CallView(title: call.title, session: call.session) {
                ui.active = nil
            }
        }
    }
}
#endif
