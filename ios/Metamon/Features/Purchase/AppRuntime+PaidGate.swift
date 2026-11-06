import SwiftUI
import MetamonDSL
import MetamonRenderer
import MetamonServices

#if !APP_STORE_REVIEW
/// チャットの確認カードを通らない経路の有料機能(#414)。判定は `PaidGate`(純粋関数)。
/// - 生成の後: 即生成(チャット OFF・「別のアプリ」)・予備文の「このまま作る / このまま反映」・圏外の保留・状態カードの再送・
///   「この機能なしで作る」でも AI が共有にした時。共有の記録が新しく入り Pro でなければ課金画面。
///   閉じたら(買わなかったら)共有を外す修正を送る。外せなければ前の版に戻す(新規作成なら初期画面へ)。
/// - 元に戻す / やり直す: 共有の版に戻った時も同じ。閉じたら戻す前の版へ。
extension AppRuntime {
    func gatePaidGeneration(before: AppDocument) async {
        guard PaidGate.generatedNeedsPaywall(before: before.dsl, after: dsl, isPro: proGate.isPro, enforced: proGate.isEnforced) else { return }
        let appId = document.appId
        let version = document.version
        print("[Metamon] IAP: generated app uses shared data without Pro → paywall (version=\(version))")
        let personalData = publicDataPrompt // #253 の確認は課金画面の後(買った時だけ)に出す
        publicDataPrompt = nil
        paidGatePending = true // H7: 要約バナー・確認は課金画面が閉じてから
        let pro = await requestProOverSheets(closeNote: L10n.tr("閉じると、共有の機能を外して自分だけの記録にします。"))
        paidGatePending = false
        guard document.appId == appId, document.version == version else { return } // 待っている間に切り替えた・直した
        if pro {
            publicDataPrompt = personalData
            return
        }
        await dropSharedData(fallback: before)
    }

    func gatePaidRestore(before: AppDocument) async {
        guard PaidGate.generatedNeedsPaywall(before: before.dsl, after: dsl, isPro: proGate.isPro, enforced: proGate.isEnforced) else { return }
        let appId = document.appId
        let version = document.version
        print("[Metamon] IAP: restored version uses shared data without Pro → paywall")
        let pro = await requestProOverSheets(closeNote: L10n.tr("閉じると、元の版に戻します。"))
        guard !pro, document.appId == appId, document.version == version else { return }
        await restoreDocument(before)
        summaryBanner = SummaryBanner(text: L10n.tr("共有の機能は Tailor It Pro が必要なため、元の版に戻しました"), highlight: nil)
    }

    /// 買わなかった時: 共有を外す修正を送る(チェックなし)。それでも共有が残れば前の版へ。
    private func dropSharedData(fallback before: AppDocument) async {
        print("[Metamon] IAP: paywall closed → drop shared data")
        await send(PaidGate.dropSharedDataInstruction(english: LanguagePreference.shared.code == "en"), gatePaid: false)
        guard PaidGate.usesSharedData(dsl), proGate.needsPaywall else {
            summaryBanner = SummaryBanner(text: L10n.tr("共有の機能を外して、自分だけの記録にしました"), highlight: nil, action: canUndo ? .undo : nil)
            return
        }
        print("[Metamon] IAP: shared data still present → revert")
        publicDataPrompt = nil
        if before.dsl.isVoiceOnly {
            startFresh(.home)
        } else {
            await restoreDocument(before)
        }
        summaryBanner = SummaryBanner(text: L10n.tr("共有の機能は Tailor It Pro が必要なため、元に戻しました"), highlight: nil)
    }

    /// 前の版に戻す。サーバーがあれば restore(新しい版になる)、無ければ端末の DSL をそのまま当てる。
    func restoreDocument(_ doc: AppDocument) async {
        guard doc.appId == document.appId else { return }
        if services.isLive {
            do {
                let restored = try await services.appConfig.restore(appId: doc.appId, toVersion: doc.version)
                apply(restored)
                startSync()
                return
            } catch {
                print("[Metamon] IAP: restore v\(doc.version) failed: \(error)")
            }
        }
        apply(doc)
    }

    /// サーバーが生成・直しの保存を 402 `pro_required` で止めた(#316・有料の機能が入る)。何も保存されていない。
    /// 課金画面を出し、買えば(サーバーへの取引の送信を待って)同じ依頼をもう一度送る。閉じたら理由をバナーで伝える。
    func presentProRequired(_ pro: ProRequired, transcript: String, gatePaid: Bool) async {
        let appId = document.appId
        guard !proGate.isPro else {
            // 端末では Pro なのにサーバーが知らない(取引の送信がまだ・失敗)。課金画面は出さず、送り直しもしない(繰り返さない)
            print("[Metamon] IAP: server refused a paid save although this device is Pro")
            keepChatAfterStop = true // 会話は残す(少し待って「これでいく」でまた送れる・H3)
            summaryBanner = SummaryBanner(text: L10n.tr("購入の確認がまだサーバーに届いていません。少し待ってから、もう一度話しかけてください"), highlight: nil)
            return
        }
        print("[Metamon] IAP: server refused a paid save (\(pro.features.map(\.id))) → paywall")
        let wasOpen = showVoiceSheet
        showVoiceSheet = false
        let bought = await proGate.requestPro(.paidFeature(pro.features), closeNote: L10n.tr("閉じると、この内容では作りません。"),
                                              after: .milliseconds(wasOpen ? 550 : 350))
        guard document.appId == appId else { return }
        guard bought else {
            // 新規作成の対話中は会話を残し、確認カード(購入して作る / この機能なしで作る)に戻す(H3)
            if holdForProRequired(pro, transcript: transcript) { return }
            summaryBanner = SummaryBanner(text: pro.notSavedMessage, highlight: nil)
            return
        }
        await proGate.store.serverSyncTask?.value // サーバーが Pro を知ってから送り直す
        await send(transcript, gatePaid: gatePaid)
    }

    /// 共有の記録の書き込みが 402(#316)。作成者なら課金画面、参加者は払っても直らないので案内だけ。
    func presentProRequiredWrite(_ pro: ProRequired) {
        guard !pro.creatorNeedsPro else {
            toast = pro.writeMessage
            return
        }
        print("[Metamon] IAP: shared write needs Pro → paywall")
        Task { _ = await proGate.requestPro(.paidFeature(pro.features), closeNote: L10n.tr("閉じると、共有のデータは見るだけになります。")) }
    }

    /// 音声シートが開いていれば閉じ終わるのを待ってから、root の課金画面を出す。
    private func requestProOverSheets(closeNote: String) async -> Bool {
        let wasOpen = showVoiceSheet
        showVoiceSheet = false
        return await proGate.requestPro(.paidFeature([]), closeNote: closeNote, after: .milliseconds(wasOpen ? 550 : 350))
    }
}
#else
/// 審査用ビルド(APP_STORE_REVIEW・#450 G1)は App 内課金を持たない。共有の記録も作れない(#451 G2・backend が private に直す)ので、
/// 生成・元に戻す / やり直すの後に課金画面は出さない。
extension AppRuntime {
    func gatePaidGeneration(before: AppDocument) async {}
    func gatePaidRestore(before: AppDocument) async {}

    /// 402 `pro_required`(#316)。この版は有料の機能を作らない(backend が共有を private に直し、serverRules を作らない)ので
    /// 当たらない想定。当たっても課金画面は無いので、理由を伝えるだけ。
    func presentProRequired(_ pro: ProRequired, transcript: String, gatePaid: Bool) async {
        print("[Metamon] review build got pro_required (\(pro.features.map(\.id)))")
        summaryBanner = SummaryBanner(text: pro.notSavedMessage, highlight: nil)
    }

    func presentProRequiredWrite(_ pro: ProRequired) {
        toast = pro.writeMessage
    }
}
#endif
