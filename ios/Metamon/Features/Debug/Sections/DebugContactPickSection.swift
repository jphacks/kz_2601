#if DEBUG && METAMON_STG
import MetamonRenderer
import SwiftUI

/// 開発用メニュー: 連絡先から 1 人選ぶ口(#2217)を部品なしで試す。DEBUG の版だけ。
/// `CNContactPicker` を出すだけで `CNContactStore` は触らないので、許可のダイアログは出ない。
final class DebugContactPickSection: DebugMenuSection {
    override class var order: Int { 250 }
    override class var title: String { "連絡先から 1 人選ぶ" }
    override class var footer: String? {
        "CNContactPicker を出し、選んだ 1 人から名前・電話・メール・誕生日を取り出します。連絡先全体は読まないので許可のダイアログは出ません。"
    }
    @MainActor override class func makeBody() -> AnyView { AnyView(DebugContactPickRows()) }
}

private struct DebugContactPickRows: View {
    @State private var picking = false
    @State private var picked: PickedContact?

    var body: some View {
        Button {
            picking = true
        } label: {
            DebugKnobLabel(title: "連絡先から 1 人選ぶ", effect: .immediate)
        }
        .accessibilityIdentifier("debugMenu.contactPick.open")
        .sheet(isPresented: $picking) {
            ContactPickerSheet { picked = $0 }
        }
        if let picked {
            ForEach(ContactItem.allCases, id: \.rawValue) { item in
                LabeledContent {
                    Text(verbatim: picked[item] ?? "-")
                        .font(.footnote.monospaced())
                        .textSelection(.enabled)
                } label: {
                    Text(verbatim: item.rawValue)
                }
            }
            .accessibilityIdentifier("debugMenu.contactPick.result")
        }
    }
}
#endif
