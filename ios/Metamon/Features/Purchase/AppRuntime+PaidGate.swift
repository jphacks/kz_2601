import SwiftUI
import MetamonDSL
import MetamonRenderer
import MetamonServices

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
        let pro = await requestProOverSheets(closeNote: L10n.tr("閉じると、共有の機能を外して自分だけの記録にします。"))
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

    /// 音声シートが開いていれば閉じ終わるのを待ってから、root の課金画面を出す。
    private func requestProOverSheets(closeNote: String) async -> Bool {
        let wasOpen = showVoiceSheet
        showVoiceSheet = false
        return await proGate.requestPro(.paidFeature([]), closeNote: closeNote, after: .milliseconds(wasOpen ? 550 : 350))
    }
}
