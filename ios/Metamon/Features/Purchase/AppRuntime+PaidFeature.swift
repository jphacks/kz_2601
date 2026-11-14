// 審査用ビルド(APP_STORE_REVIEW・#450 G1)は App 内課金を持たない(有料機能の確認カードを出さない)。
#if !APP_STORE_REVIEW
import SwiftUI
import MetamonServices

/// 有料機能の確認カード(#77)と課金画面(#414)。チャットの「これでいく」(ready / 案)の時、backend の `requiresPro` を見て
/// 確認カードだけを出す(課金画面は自動で開かない・UX 点検 H4 / オーナーの決定 U3)。カードで「Pro にする」/「この機能なしで作る」/
/// 「やめる」を選ぶ。#414「必ず課金画面を通す」は「Pro にする」を押した時に課金画面を開くことで満たす。
/// 課金画面で買えば有料機能込みで作り、閉じたらカードに戻る(閉じただけでは何も作らない)。
/// - Pro 会員・課金の制限の無効化中(Debug の既定。`METAMON_IAP_ENFORCE=1` で有効)は出さない(`PaidFeatureCheck`)
/// - チャットを通らない生成は、生成の後に DSL で判定する(`AppRuntime+PaidGate.swift`)
extension AppRuntime {
    /// 確認が要るならカードを出して true(呼び出し側は生成しない)。課金画面はカードの「Pro にする」で開く(H4)。
    func holdForPaidConfirm(_ confirmation: PaidConfirmation?) -> Bool {
        guard let confirmation else { return false }
        print("[Metamon] IAP: paid feature confirm features=\(confirmation.features.map(\.id)) mode=\(chatMode.rawValue)")
        withAnimation(.spring(duration: 0.4, bounce: 0.15)) {
            paidConfirmMessage = nil
            paidConfirm = confirmation
        }
        return true
    }

    /// 「Pro にする」: 課金画面を開き、Pro になって閉じたら有料機能込みの文で生成。買わずに閉じたらカードに残る。
    func purchaseAndBuild() async {
        guard let confirmation = paidConfirm, !paidConfirmBusy else { return }
        paidConfirmBusy = true
        let pro = await proGate.requestPro(.paidFeature(confirmation.features), placement: .chat,
                                           closeNote: L10n.tr("閉じても、まだ作りません。「この機能なしで作る」か「やめる」を選べます。"))
        paidConfirmBusy = false
        guard pro, paidConfirm == confirmation else { return } // 買わなかった / 待っている間に閉じた・話し直した
        print("[Metamon] IAP: pro → build with paid features")
        await proGate.store.serverSyncTask?.value // サーバーが Pro を知ってから作る(402 の後に買った時・H3)
        await runFromChat(confirmation.paidText)
    }

    /// 「この機能なしで作る」: 有料機能を抜いた文で生成する。
    /// カードはここで消さない(完成時に apply → resetChat が消す)。カードの撤去と生成中表示の挿入が同じ更新に重なると、
    /// 遅い環境(CI)で GeneratingView が完成まで描かれなかった(#200 と同じ形)。「Pro にする」もカードを残したまま作る
    func buildWithoutPaidFeatures() async {
        guard let confirmation = paidConfirm, morphJob == nil else { return }
        print("[Metamon] IAP: build without paid features=\(confirmation.features.map(\.id))")
        await runFromChat(confirmation.freeText)
        if paidConfirm == confirmation { clearPaidConfirm() } // 作らずに終わった時の保険(通常は resetChat が消している)
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
#else
extension AppRuntime {
    /// 確認カードは無い(審査用ビルド・#450 G1)。チャットのリセット時の呼び出しを残すための空の実装。
    func clearPaidConfirm() {}
}
#endif
