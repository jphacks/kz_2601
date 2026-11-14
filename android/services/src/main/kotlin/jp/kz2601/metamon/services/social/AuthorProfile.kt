package jp.kz2601.metamon.services.social

import jp.kz2601.metamon.services.consent.KeyValueStore

/**
 * 共有アプリへの投稿に出す表示名(iOS の Features/Settings/AuthorProfile.swift・#250)。本人が決めた名前だけを使う。
 * Google アカウントの氏名・メールアドレス・端末名は使わない。未設定なら送らず、backend が「匿名」を付ける。
 */
class AuthorProfile(private val store: KeyValueStore) {
    /** 投稿に付ける名前。空白だけ・未設定は null。 */
    var name: String?
        get() = normalized(store.getString(NAME_KEY).orEmpty())
        set(value) = store.putString(NAME_KEY, normalized(value.orEmpty()).orEmpty())

    /** 初めての共有投稿で一度だけ名前を聞いたか(設定で名前を決めた時も済みにする)。 */
    var asked: Boolean
        get() = store.getBoolean(ASKED_KEY) == true
        set(value) = store.putBoolean(ASKED_KEY, value)

    /** 名前を聞いた答え。null は「匿名で投稿」(名前は変えない)。 */
    fun answer(name: String?) {
        if (name != null) this.name = name
        asked = true
    }

    /** アカウント削除・サインアウトで消す。 */
    fun clear() {
        store.remove(NAME_KEY)
        store.remove(ASKED_KEY)
    }

    companion object {
        const val NAME_KEY = "authorProfile.name"
        const val ASKED_KEY = "authorProfile.asked"

        /** backend の上限(64 文字)より短く。一覧の 1 行に収まる長さ */
        const val MAX_LENGTH = 20

        fun normalized(raw: String): String? = raw.trim().take(MAX_LENGTH).ifEmpty { null }
    }
}
