package jp.kz2601.metamon.voice.stage

import android.content.Context

/**
 * 完成の直後に出るお知らせ(#894・iOS の `PostBuildNotice`・H7)。同時に出さず、優先順位の高いものから 1 つずつ(前のが閉じたら次)出す。
 * Android にまだ無いもの(課金画面・公開の記録の確認・保存期間の確認・引き継ぎの案内)も順番だけは iOS と揃えておく。
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
    }
}

/** 「こう話すと直せます」を出したかを端末に残す(iOS の `FirstRunTutorialStore` の editHint の分)。 */
class EditHintStore(private val read: () -> Boolean, private val write: () -> Unit) {
    /** 初期画面からの作成の直後で、まだ出していなければ出す */
    fun shouldShow(createdFromInitial: Boolean): Boolean = createdFromInitial && !read()

    fun markShown() = write()

    companion object {
        private const val PREFS = "metamon.tutorial"
        private const val KEY = "editHintShown"

        fun of(context: Context): EditHintStore {
            val prefs = context.applicationContext.getSharedPreferences(PREFS, Context.MODE_PRIVATE)
            return EditHintStore(read = { prefs.getBoolean(KEY, false) }, write = { prefs.edit().putBoolean(KEY, true).apply() })
        }
    }
}
