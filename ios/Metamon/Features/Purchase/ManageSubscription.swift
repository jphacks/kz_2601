import SwiftUI
#if !APP_STORE_REVIEW
import StoreKit
#endif

/// 「サブスクリプションを管理」(OS の購読管理の画面を開く・#326 / 3.1.2)をどこに出すか。
/// 審査用ビルド(APP_STORE_REVIEW・#450 G1)は App 内課金を持たないので、どこにも出さない(nil)。
enum ManageSubscriptionPlacement: Equatable {
    /// Pro の時: プランの状態のすぐ下(目立つ位置)
    case prominent
    /// Pro でない時: 「購入を復元」の近く(下の方)
    case nearRestore

    static func placement(isPro: Bool) -> ManageSubscriptionPlacement? {
        #if APP_STORE_REVIEW
        return nil
        #else
        return isPro ? .prominent : .nearRestore
        #endif
    }
}

#if !APP_STORE_REVIEW
/// OS の購読管理の画面(StoreKit 2 の `manageSubscriptionsSheet`)を開くボタン。解約・プランの確認は Apple の画面で行う。
struct ManageSubscriptionButton: View {
    var accessibilityID: String
    @State private var presented = false

    var body: some View {
        Button {
            presented = true
        } label: {
            Label("サブスクリプションを管理", systemImage: "creditcard")
        }
        .accessibilityIdentifier(accessibilityID)
        .manageSubscriptionsSheet(isPresented: $presented)
    }
}
#endif
