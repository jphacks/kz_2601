import SwiftUI

/// 設定の「アクションボタン」の案内(#1993)。iOS 18 はコントロールをそのまま割り当てられるので、iOS 17 の端末だけに出す。
/// iOS 17 はショートカットの「アプリを開く」(OpenTailorAppIntent)を割り当ててもらう。
struct ActionButtonHintSection: View {
    var body: some View {
        if #unavailable(iOS 18) {
            Section(L10n.tr("アクションボタン")) {
                Text(Self.hint)
                    .font(.footnote)
                    .foregroundStyle(.secondary)
                    .accessibilityIdentifier("settings.actionButtonHint")
            }
        }
    }

    static var hint: String {
        L10n.pick(
            L10n.tr("アクションボタンのある iPhone では、設定アプリの「アクションボタン」で「ショートカット」を選び、Tailor It の「アプリを開く」を割り当てると、押すだけで作ったアプリを開けます。"),
            review: L10n.tr("アクションボタンのある iPhone では、設定アプリの「アクションボタン」で「ショートカット」を選び、Tailor It の「記録帳を開く」を割り当てると、押すだけで記録帳を開けます。")
        )
    }
}
