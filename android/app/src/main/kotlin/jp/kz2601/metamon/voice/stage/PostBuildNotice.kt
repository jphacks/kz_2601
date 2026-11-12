package jp.kz2601.metamon.voice.stage

/**
 * 完成の直後に出るお知らせ(#894・iOS の `PostBuildNotice`・H7)。同時に出さず、優先順位の高いものから 1 つずつ(前のが閉じたら次)出す。
 * 順番と待つ条件は iOS の `AppRuntime.pendingNotices` と同じ。課金画面・公開の記録の確認・引き継ぎの案内は [PostBuildWaiting] の印で順番に入る(#1198)。
 */
enum class PostBuildNotice {
    /** 共有の記録が入った版の課金画面 */
    PAYWALL,

    /** 要約バナー(「元に戻す」付き)。「できた」をまず見せる */
    SUMMARY,

    /** 公開の記録に個人の情報が入った確認 */
    PUBLIC_DATA,

    /** 記録の保存期間の確認 */
    RETENTION,

    /** 「こう話すと直せます」(最初のアプリの直後に 1 回だけ) */
    EDIT_HINT,

    /** 引き継ぎ(匿名 → アカウント)の案内 */
    ACCOUNT_LINK,
    ;

    companion object {
        /** 待っているお知らせのうち、いま出すもの(無ければ null)。 */
        fun current(pending: Set<PostBuildNotice>): PostBuildNotice? = pending.minOrNull()

        /** 待っているお知らせ(iOS の `AppRuntime.pendingNotices`)。 */
        fun pending(
            summary: Boolean,
            retention: Boolean = false,
            editHint: Boolean = false,
            waiting: PostBuildWaiting = PostBuildWaiting.NONE,
        ): Set<PostBuildNotice> = buildSet {
            if (waiting.paywall) add(PAYWALL)
            if (summary) add(SUMMARY)
            if (waiting.publicData) add(PUBLIC_DATA)
            if (retention) add(RETENTION)
            if (editHint) add(EDIT_HINT)
            if (waiting.accountLink) add(ACCOUNT_LINK)
        }
    }
}

/**
 * 要約・保存期間・直し方の例のほかに待っているお知らせ(iOS の `pendingNotices` の残り)。
 * @param paywall 課金画面を出している(iOS の `paidGatePending || proGate.paywall?.placement == .root`)
 * @param publicData 公開の記録に個人の情報が入った確認を待っている(iOS の `publicDataPrompt != nil`)
 * @param accountLink 引き継ぎの案内を待っている(iOS の `showLinkPrompt`)
 */
data class PostBuildWaiting(val paywall: Boolean = false, val publicData: Boolean = false, val accountLink: Boolean = false) {
    companion object {
        val NONE = PostBuildWaiting()
    }
}

/**
 * 「こう話すと直せます」を出したか(iOS の `FirstRunTutorialStore` の editHint の分)。
 * 置き場はチュートリアルの見た印と同じ(`FirstRunTutorialStore.editHints`・後片付けで一緒に消す)。
 */
class EditHintStore(private val read: () -> Boolean, private val write: () -> Unit) {
    /** 初期画面からの作成の直後で、まだ出していなければ出す */
    fun shouldShow(createdFromInitial: Boolean): Boolean = createdFromInitial && !read()

    fun markShown() = write()
}
