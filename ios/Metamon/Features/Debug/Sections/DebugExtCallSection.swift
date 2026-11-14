#if METAMON_STG
import SwiftUI
import MetamonServices

/// 開発用メニュー: 外の API の断り(#2001)を部品なしで出す。#2079 で設定の「つないだサービス」から移した
final class DebugExtCallSection: DebugMenuSection {
    override class var order: Int { 200 }
    override class var title: String { "つないだサービス" }
    override class var footer: String? {
        "偽の provider fakeOauth に op を投げます。METAMON_MOCK=1 と METAMON_MOCK_EXT_ERROR=<code> で断りを仕込めます。"
    }
    @MainActor override class func makeBody() -> AnyView { AnyView(DebugExtCallRows()) }
}

private struct DebugExtCallRows: View {
    @Environment(\.services) private var services
    @State private var denial: ExtDenial?
    @State private var message: String?

    var body: some View {
        Button {
            Task {
                let (d, text) = await DebugExtDenial.call(services.external)
                message = text
                denial = d
            }
        } label: {
            DebugKnobLabel(title: "試験用のサービスを呼ぶ", effect: .immediate)
        }
        .accessibilityIdentifier("debugMenu.ext.call")
        .extDenialSheets($denial) { message = $0 }
        if let message {
            Text(verbatim: message)
                .font(.footnote)
                .foregroundStyle(.secondary)
                .accessibilityIdentifier("debugMenu.ext.message")
        }
    }
}
#endif
