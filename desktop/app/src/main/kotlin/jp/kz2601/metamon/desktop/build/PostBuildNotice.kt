package jp.kz2601.metamon.desktop.build

/**
 * 完成の直後に出るお知らせ(Android の `PostBuildNotice`・iOS の `PostBuildNotice` と同じ順番)。
 * 同時に出さず、番号の小さい(優先の高い)ものから 1 つずつ出す。描画はしない。
 */
enum class PostBuildNotice {
    /** 共有の記録が入った版の課金画面 */
    PAYWALL,

    /** 要約バナー(「元に戻す」付き) */
    SUMMARY,

    /** 公開の記録に個人の情報が入った確認 */
    PUBLIC_DATA,

    /** 記録の保存期間の確認 */
    RETENTION,

    /** 「こう頼むと直せます」(最初のアプリの直後に 1 回だけ) */
    EDIT_HINT,

    /** 引き継ぎ(匿名 → アカウント)の案内 */
    ACCOUNT_LINK,
    ;

    companion object {
        /** 待っているお知らせのうち、いま出すもの(無ければ null)。 */
        fun current(pending: Set<PostBuildNotice>): PostBuildNotice? = pending.minOrNull()

        /** 待っているお知らせの集まり。 */
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

/** 要約・保存期間・直し方のほかに待っているお知らせ。 */
data class PostBuildWaiting(
    val paywall: Boolean = false,
    val publicData: Boolean = false,
    val accountLink: Boolean = false,
) {
    companion object {
        val NONE = PostBuildWaiting()
    }
}
