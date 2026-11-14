package jp.kz2601.metamon.widget

import android.content.Context
import jp.kz2601.metamon.services.consent.InMemoryKeyValueStore
import jp.kz2601.metamon.services.consent.KeyValueStore
import jp.kz2601.metamon.services.consent.SharedPreferencesKeyValueStore

/**
 * ウィジェットの中身を 1 件だけ置く所(iOS の App Group の `UserDefaults`)。端末だけ(allowBackup=false)。
 * サインアウト・端末のデータの後片付けと、表示中のアプリを消した時に消す。
 */
class WidgetSnapshotStore(private val store: KeyValueStore = InMemoryKeyValueStore()) {
    fun load(): WidgetSnapshot? = WidgetSnapshot.decode(store.getString(KEY))

    fun save(snapshot: WidgetSnapshot) = store.putString(KEY, snapshot.encode())

    fun clear() = store.remove(KEY)

    /** そのアプリの分が出ていれば消す(アプリの削除)。消したら true。 */
    fun clearIfShowing(appId: String): Boolean {
        if (load()?.appId != appId) return false
        clear()
        return true
    }

    companion object {
        private const val PREFS = "metamon-widget"
        private const val KEY = "snapshot"

        fun of(context: Context) = WidgetSnapshotStore(
            SharedPreferencesKeyValueStore(context.applicationContext.getSharedPreferences(PREFS, Context.MODE_PRIVATE)),
        )
    }
}
