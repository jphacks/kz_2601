// 審査用ビルド(APP_STORE_REVIEW・#450 G1)は App 内課金を持たない(端末の無料枠も 402 の課金画面も無い)。
#if !APP_STORE_REVIEW
import SwiftUI
import MetamonServices

/// 無料枠切れで止めた新規作成(H3)。会話は消さずに残し、Pro を買う・枠が戻ったら同じ依頼で続きから作る。
struct HeldBuild: Equatable {
    /// 作るはずだった依頼文(ready の transcript / 案の instruction)
    var text: String
}

/// 上限・402 `pro_required` で作らずに止めた時、聞き返しに答えた会話を消さない(H3)。
/// - 無料枠切れ: 課金画面で買えばそのまま作る(`sendMetered`)。閉じたら会話を残し、チャットに「続きから作る」のカード
/// - 402: 課金画面で買えば同じ依頼を送り直す(`presentProRequired`)。閉じたら会話を残し、確認カード(購入して作る / この機能なしで作る)
extension AppRuntime {
    /// 無料枠切れで送らなかった。対話中なら会話を残してカードを出し、そうでなければ入力欄に依頼を戻す(言い直さなくてよい)。
    func holdBuild(_ text: String) {
        print("[Metamon] IAP: limit reached → keep the conversation and offer to continue")
        guard isChatting else {
            composerDraft = text
            return
        }
        keepChatAfterStop = true
        withAnimation(.spring(duration: 0.4, bounce: 0.15)) { heldBuild = HeldBuild(text: text) }
    }

    /// 「続きから作る」: 同じ依頼で作る。まだ枠が無く Pro でもなければ、もう一度課金画面を出す(閉じたらカードに戻る)。
    func continueHeldBuild() async {
        guard let held = heldBuild, morphJob == nil else { return }
        heldBuild = nil
        print("[Metamon] IAP: continue held build")
        await runFromChat(held.text)
    }

    /// カードを閉じてチャットに戻る(会話は残る。「これでいく」からまた作れる)。
    func dismissHeldBuild() {
        withAnimation(.easeInOut(duration: 0.25)) { heldBuild = nil }
    }

    /// 402 で課金画面を閉じた時、新規作成の対話中なら会話を残して確認カードを出す。出したら true。
    func holdForProRequired(_ pro: ProRequired, transcript: String) -> Bool {
        guard dsl.isVoiceOnly, isChatting else { return false }
        print("[Metamon] IAP: pro_required closed → keep the conversation with the paid card")
        keepChatAfterStop = true
        withAnimation(.spring(duration: 0.4, bounce: 0.15)) {
            paidConfirm = PaidConfirmation(features: pro.features, paidText: transcript,
                                           freeText: transcript + "\n" + PaidFeatureCheck.fallbackFreeSuffix)
            paidConfirmMessage = pro.notSavedMessage
        }
        return true
    }

    /// カードの見出しに出す 1 日の無料回数。
    var dailyFreeCreations: Int { proGate.quota.dailyFreeLimit }
}
#endif
