#if METAMON_STG
import SwiftUI

/// 開発用メニュー: 課金(#2079 で設定の「Tailor It Pro」から移した)
final class DebugProSection: DebugMenuSection {
    override class var order: Int { 100 }
    override class var title: String { "課金" }
    override class var footer: String? {
        "既定は Debug の版で制限なし、Staging の版で制限あり。METAMON_IAP_ENFORCE=1 で起動すると、ここに関係なく制限が効きます。"
    }
    @MainActor override class func makeBody() -> AnyView { AnyView(DebugProRows()) }
    @MainActor override class func reset() { ProGate.shared.limitDisabled = ProGate.defaultLimitDisabled }
}

private struct DebugProRows: View {
    var body: some View {
        Toggle(isOn: Bindable(ProGate.shared).limitDisabled) {
            DebugKnobLabel(title: "課金の制限を無効にする(デモ用・回数と有料機能)", effect: .immediate)
        }
        .tint(Theme.Colors.brandA)
        .accessibilityIdentifier("debugMenu.pro.limitDisabled")
    }
}
#endif
