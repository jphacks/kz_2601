package jp.kz2601.metamon.account

import jp.kz2601.metamon.services.consent.KeyValueStore
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import java.util.UUID

/**
 * 「作ったアプリを残しておきませんか」の案内(#1316・iOS の `AccountLinkPrompt`)。端末ごとに 1 回だけ。
 * 匿名の人が最初のアプリを作った起動を覚えておき([offerIfNeeded])、次の起動で出す([presentDeferredIfNeeded])。
 * 最初のアプリの直後には出さない(H7: 完成直後のお知らせを減らす)。
 *
 * @param launch この起動の id(プロセスごと)。同じ起動の予約はまだ出さない
 */
class AccountLinkPrompt(private val store: KeyValueStore, private val launch: String = CURRENT_LAUNCH) {
    private val _visible = MutableStateFlow(false)

    /** 案内を出す番を待っている(実際に出すのは他のシート・お知らせが閉じてから・`AccountLinkPromptHost`) */
    val visible: StateFlow<Boolean> = _visible.asStateFlow()

    fun shouldOffer(): Boolean = store.getBoolean(KEY) != true

    /** 最初のアプリを作った: この起動の id で予約する(既に予約があれば前のを残す)。 */
    fun offerIfNeeded(isAnonymous: Boolean) {
        if (!isAnonymous || !shouldOffer() || store.getString(PENDING_KEY) != null) return
        store.putString(PENDING_KEY, launch)
    }

    /** 前の起動で予約した案内があるか(同じ起動の予約はまだ出さない)。 */
    fun isDue(): Boolean = store.getString(PENDING_KEY)?.let { it != launch } == true

    /**
     * 起動時: 前の起動で予約した案内があれば 1 回だけ出す(出した扱いにして [visible] を立てる)。
     * もう Google・Apple に引き継いだ人は予約だけ消す。出すなら true。
     */
    fun presentDeferredIfNeeded(isAnonymous: Boolean): Boolean {
        if (!isDue()) return false
        if (!isAnonymous || !shouldOffer()) {
            store.remove(PENDING_KEY)
            return false
        }
        markOffered()
        _visible.value = true
        return true
    }

    fun dismiss() {
        _visible.value = false
    }

    fun markOffered() {
        store.putBoolean(KEY, true)
        store.remove(PENDING_KEY)
    }

    /** 後片付け(#841)。iOS は `LocalWipe.clearDefaults` でこの鍵も消すので、次の人には出し直す。 */
    fun forget() {
        store.remove(KEY)
        store.remove(PENDING_KEY)
        _visible.value = false
    }

    companion object {
        /** SharedPreferences の名前 */
        const val PREFS = "metamon.accountLink"

        /** 案内を出したか(iOS の UserDefaults の鍵と同じ名前) */
        const val KEY = "metamon.accountLinkPrompted"

        /** 案内を待っている(値 = 最初のアプリを作った起動の id) */
        const val PENDING_KEY = "metamon.accountLinkPending"

        /** この起動の id(プロセスごと) */
        val CURRENT_LAUNCH: String = UUID.randomUUID().toString()
    }
}
