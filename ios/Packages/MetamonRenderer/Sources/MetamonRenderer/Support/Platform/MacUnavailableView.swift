#if os(macOS)
import SwiftUI

/// Mac でまだ作っていない / 使えない部品の仮の枠(mac-004)。ビルドを通すためのもので、
/// 正式な「この端末では使えません」の枠と対応表は mac-025、各部品の Mac 版は mac-027 以降で作る。
struct MacUnavailableView: View {
    /// 何が使えないか(例: 「カメラ」)
    let feature: String

    var body: some View {
        ContentUnavailableView {
            Label(L("この Mac では使えません"), systemImage: "desktopcomputer.trianglebadge.exclamationmark")
        } description: {
            Text(feature)
        }
        .accessibilityElement(children: .combine)
    }
}
#endif
