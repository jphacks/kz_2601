package jp.kz2601.metamon

import android.content.Context
import jp.kz2601.metamon.services.local.CurrentAppStore
import java.io.File

/** 端末の今のアプリの appId(iOS の `DeviceAppID`)の置き場を作る。中身は [CurrentAppStore]。 */
object DeviceAppId {
    private const val PREFS = "metamon"
    private const val LEGACY_KEY = "metamon.appId"

    /**
     * `filesDir/<dir>/current.json` に置く。前の版が SharedPreferences に入れた appId があれば 1 回だけ移す
     * (同じアプリを開き直せるように)。
     */
    fun store(context: Context, dir: String, migrateLegacy: Boolean = true): CurrentAppStore {
        val directory = File(context.filesDir, dir)
        val store = CurrentAppStore(directory)
        if (migrateLegacy && !File(directory, "current.json").exists()) {
            val prefs = context.applicationContext.getSharedPreferences(PREFS, Context.MODE_PRIVATE)
            prefs.getString(LEGACY_KEY, null)?.takeIf { it.isNotEmpty() }?.let {
                store.set(it)
                prefs.edit().remove(LEGACY_KEY).apply()
            }
        }
        return store
    }
}
