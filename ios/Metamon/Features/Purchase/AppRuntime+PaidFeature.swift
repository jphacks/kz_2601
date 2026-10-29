import SwiftUI
import MetamonServices

/// 有料機能の確認カード(#77)と課金画面(#414)。チャットの「これでいく」(ready / 案)の時、backend の `requiresPro` を見て
/// 確認カードを出し、同時に課金画面(`PaywallSheet`)を開く。買えば有料機能込みで作る。閉じたらカードに戻り、
/// 「この機能なしで作る」/「やめる」/ もう一度「購入して作る」(= 課金画面)を選ぶ。閉じただけでは何も作らない。
/// - Pro 会員・課金の制限の無効化中(Debug の既定。`METAMON_IAP_ENFORCE=1` で有効)は出さない(`PaidFeatureCheck`)
/// - チャットを通らない生成は、生成の後に DSL で判定する(`AppRuntime+PaidGate.swift`)
extension AppRuntime {
    /// 確認が要るならカードと課金画面を出して true(呼び出し側は生成しない)。
    func holdForPaidConfirm(_ confirmation: PaidConfirmation?) -> Bool {
        guard let confirmation else { return false }
        print("[Metamon] IAP: paid feature confirm features=\(confirmation.features.map(\.id)) mode=\(chatMode.rawValue)")
        withAnimation(.spring(duration: 0.4, bounce: 0.15)) {
            paidConfirmMessage = nil
            paidConfirm = confirmation
        }
        Task { await purchaseAndBuild() }
        return true
    }

    /// 「購入して作る」: 課金画面を開き、Pro になって閉じたら有料機能込みの文で生成。買わずに閉じたらカードに残る。
    func purchaseAndBuild() async {
        guard let confirmation = paidConfirm, !paidConfirmBusy else { return }
        paidConfirmBusy = true
        let pro = await proGate.requestPro(.paidFeature(confirmation.features), placement: .chat,
                                           closeNote: L10n.tr("閉じても、まだ作りません。「この機能なしで作る」か「やめる」を選べます。"))
        paidConfirmBusy = false
        guard pro, paidConfirm == confirmation else { return } // 買わなかった / 待っている間に閉じた・話し直した
        print("[Metamon] IAP: pro → build with paid features")
        await runFromChat(confirmation.paidText)
    }

    /// 「この機能なしで作る」: 有料機能を抜いた文で生成する。
    func buildWithoutPaidFeatures() async {
        guard let confirmation = paidConfirm else { return }
        print("[Metamon] IAP: build without paid features=\(confirmation.features.map(\.id))")
        clearPaidConfirm()
        await runFromChat(confirmation.freeText)
    }

    /// カードを閉じてチャットに戻る(「やめる」)。
    func dismissPaidConfirm() {
        withAnimation(.easeInOut(duration: 0.25)) { clearPaidConfirm() }
    }

    func clearPaidConfirm() {
        paidConfirm = nil
        paidConfirmMessage = nil
        proGate.dismissPaywall(placement: .chat)
    }

    /// カードの見出しに出す月額(取れなければ ¥500)。
    var proMonthlyPrice: String { proGate.store.offer?.displayPrice ?? ProOffer.mockMonthly.displayPrice }
}
