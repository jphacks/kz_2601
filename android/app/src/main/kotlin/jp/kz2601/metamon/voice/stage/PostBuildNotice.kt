package jp.kz2601.metamon.voice.stage

/**
 * 完成の直後に出るお知らせ(#894・iOS の `PostBuildNotice`・H7)。同時に出さず、優先順位の高いものから 1 つずつ(前のが閉じたら次)出す。
 * Android にまだ無いもの(課金画面・公開の記録の確認)も順番だけは iOS と揃えておく。
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

        /** Android で待っているお知らせ(iOS の `AppRuntime.pendingNotices` の Android にある分)。 */
        fun pending(
            summary: Boolean,
            retention: Boolean = false,
            editHint: Boolean = false,
            accountLink: Boolean = false,
        ): Set<PostBuildNotice> = buildSet {
            if (summary) add(SUMMARY)
            if (retention) add(RETENTION)
            if (editHint) add(EDIT_HINT)
            if (accountLink) add(ACCOUNT_LINK)
        }
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
