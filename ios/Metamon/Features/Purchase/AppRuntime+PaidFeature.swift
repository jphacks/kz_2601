import SwiftUI
import MetamonServices

/// 有料機能の確認カード(#77)。チャットの「これでいく」(ready / 案)の時、backend の `requiresPro` を見て
/// 「〇〇には Tailor It Pro が必要です」→「購入して作る」/「この機能なしで作る」を選んでもらう。
/// - Pro 会員・回数制限の無効化中(Debug の既定。`METAMON_IAP_ENFORCE=1` で有効)は出さない(`PaidFeatureCheck`)
/// - 課金の強制はしない(サーバー側のエンタイトルメントは後続・docs/ios/architecture.md §18)
extension AppRuntime {
    /// 確認が要るならカードを出して true(呼び出し側は生成しない)。
    func holdForPaidConfirm(_ confirmation: PaidConfirmation?) -> Bool {
        guard let confirmation else { return false }
        print("[Metamon] IAP: paid feature confirm features=\(confirmation.features.map(\.id)) mode=\(chatMode.rawValue)")
        withAnimation(.spring(duration: 0.4, bounce: 0.15)) {
            paidConfirmMessage = nil
            paidConfirm = confirmation
        }
        return true
    }

    /// 「購入して作る」: 購入できたら有料機能込みの文で生成。キャンセル・承認待ちはカードに残る。
    func purchaseAndBuild() async {
        guard let confirmation = paidConfirm, !paidConfirmBusy else { return }
        let store = proGate.store
        paidConfirmBusy = true
        defer { paidConfirmBusy = false }
        if !store.isPro {
            if store.monthly == nil { await store.loadProducts() }
            guard let product = store.monthly else {
                paidConfirmMessage = L10n.tr("価格を取得できませんでした。通信状態を確認してください。")
                return
            }
            do {
                switch try await store.purchase(product) {
                case .purchased: break
                case .pending:
                    paidConfirmMessage = L10n.tr("承認待ちです。承認されると自動で Pro になります。")
                    return
                case .cancelled:
                    return
                }
            } catch {
                print("[Metamon] IAP purchase (paid feature) failed: \(error)")
                paidConfirmMessage = L10n.tr("購入できませんでした。時間をおいて試してください。")
                return
            }
        }
        guard paidConfirm == confirmation else { return } // 待っている間に閉じた / 話し直した
        print("[Metamon] IAP: purchased → build with paid features")
        await runFromChat(confirmation.paidText)
    }

    /// 「この機能なしで作る」: 有料機能を抜いた文で生成する。
    func buildWithoutPaidFeatures() async {
        guard let confirmation = paidConfirm, !paidConfirmBusy else { return }
        print("[Metamon] IAP: build without paid features=\(confirmation.features.map(\.id))")
        await runFromChat(confirmation.freeText)
    }

    /// カードを閉じてチャットに戻る(「やめる」)。
    func dismissPaidConfirm() {
        guard !paidConfirmBusy else { return }
        withAnimation(.easeInOut(duration: 0.25)) { clearPaidConfirm() }
    }

    func clearPaidConfirm() {
        paidConfirm = nil
        paidConfirmMessage = nil
    }

    /// カードの見出しに出す月額(取れなければ ¥500)。
    var proMonthlyPrice: String { proGate.store.monthly?.displayPrice ?? "¥500" }
}
