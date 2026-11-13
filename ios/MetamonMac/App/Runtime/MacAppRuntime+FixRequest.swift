import Foundation
import MetamonDSL
import MetamonRenderer

/// 作ったアプリの動きの失敗(アプリの作りで直せるもの)に「直してと頼む」を付ける(#1885・iOS の `AppRuntime+FixRequest.swift`)。
/// 付けるのは 未対応のアクション・フォームの無い画面の保存ボタン・開けないリンク だけ。入力漏れ・通信・権限の失敗には付けない。
extension MacAppRuntime {
    /// 「直してと頼む」付きのトーストを出す時間(押す間を取る・iOS と同じ 6 秒)
    static let fixToastSeconds: Double = 6

    /// トーストを出し、「直してと頼む」を添える。`part` は押した部品の分かる名前(無ければ画面だけ)。
    func reportFixable(_ message: String, ctx: ActionContext?, part: String? = nil) {
        toast = message
        guard !dsl.isVoiceOnly, onAskFix != nil else { return } // 会話につながっていない所(プレビュー等)では出さない
        let title = ctx.flatMap { dsl.screen($0.route.screen)?.title }
        fixOffer = FixOffer(message: message, draft: FixRequest.draft(screenTitle: title, part: part, problem: message))
    }

    /// 保存の前段で止まった: フォームの無い画面の保存ボタンはアプリの作りの問題。入力漏れは利用者が直す(iOS と同じ)
    func reportInvalidSave(_ message: String, _ ctx: ActionContext) {
        if ctx.form == nil { reportFixable(message, ctx: ctx, part: L10n.tr("保存")) } else { fixOffer = nil; toast = message }
    }

    /// 「直してと頼む」: 依頼文を入力欄に入れて中央のアプリを直す会話を開く(送らない)。
    /// モーダルの中からは、入力を確かめてモーダルを 1 枚ずつ閉じ、最後の 1 枚が閉じ終えてから開く
    func requestFix() {
        guard let offer = fixOffer, let onAskFix else { return }
        fixOffer = nil
        toast = nil
        guard sheet != nil || fullScreen != nil else { return onAskFix(offer.draft) }
        confirmDiscard(leaving: { $0.stack == "sheet" || $0.stack == "full" }) { [weak self] in
            guard let self else { return }
            pendingFixDraft = offer.draft
            dismissTopModal() // 残りは閉じ終わるたびに `askFixAfterModal` が 1 枚ずつ(同時に閉じると onDismiss が取りこぼす)
        }
    }

    /// モーダルが閉じ終わった(`AppCanvas`)。「直してと頼む」を待っていて、まだモーダルがあれば次の 1 枚を閉じ、無くなったら会話を開く
    func askFixAfterModal() {
        guard let draft = pendingFixDraft else { return }
        guard sheet == nil, fullScreen == nil else { return dismissTopModal() }
        pendingFixDraft = nil
        onAskFix?(draft)
    }
}

extension FixRequest {
    /// Mac の案内の一文。iOS の「話しかけると」を ⌘K / ⇧⌘M で頼む言い方にする(直し方の例・頼み方のコツ)
    static let macHintKey = "うまく動かない所があったら「〇〇が動かないから直して」と ⌘K か ⇧⌘M で頼むと直せます"
    static var macHint: String { L10n.tr(String.LocalizationValue(macHintKey)) }
}
