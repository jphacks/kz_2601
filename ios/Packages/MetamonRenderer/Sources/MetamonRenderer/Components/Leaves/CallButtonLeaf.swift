// 審査用ビルド(APP_STORE_REVIEW)は通話(#447)を外す。届いた DSL の callButton は未知 type のプレースホルダになる。
#if !APP_STORE_REVIEW
import SwiftUI
import MetamonDSL

/// `callButton`(#447): 共有のアプリの参加者どうしの音声通話(`video: true` ならビデオ通話)。押すとホスト(`CallHost`)が通話の画面を開く。
/// 着信は無いので、相手もアプリを開いて同じボタンを押す。
struct CallButtonLeaf: View {
    @Environment(\.dslRuntime) private var rt
    let c: Component

    var body: some View {
        let host = rt.calls
        let video = c.video == true
        Button {
            host?.startCall(room: c.room, video: video, title: Self.title(c))
        } label: {
            Label(Self.title(c), systemImage: video ? "video.fill" : "phone.fill")
                .frame(maxWidth: .infinity)
        }
        .buttonStyle(RenderButtonStyle(kind: .primary))
        .disabled(host?.canCall != true)
        .accessibilityIdentifier("callButton.\(c.room ?? "app")")
    }

    static func title(_ c: Component) -> String {
        if let l = c.label, !l.isEmpty { return l }
        return c.video == true ? L("ビデオ通話に参加") : L("通話に参加")
    }
}
#endif
