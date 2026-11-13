import Foundation
import MetamonRenderer

/// 作ったアプリの動きの失敗(アプリの作りで直せるもの)に「直してと頼む」を付ける。
extension AppRuntime {
    /// トーストを出し、「直してと頼む」を添える。`part` は押した部品の分かる名前(無ければ画面だけ)。
    func reportFixable(_ message: String, ctx: ActionContext?, part: String? = nil) {
        toast = message
        guard !dsl.isVoiceOnly, assistiveAccess.shows(.voiceSheet) else { return } // アシスティブアクセス(#1997)では頼めない
        let title = ctx.flatMap { dsl.screen($0.route.screen)?.title }
        fixOffer = FixOffer(message: message, draft: FixRequest.draft(screenTitle: title, part: part, problem: message))
    }

    /// 「直してと頼む」: 依頼文を入力欄に入れて音声シートを開く(送らない)。
    func requestFix() {
        guard let offer = fixOffer, showsOrToast(.voiceSheet) else { return }
        fixOffer = nil
        toast = nil
        composerDraft = offer.draft
        showVoiceSheet = true
    }
}
