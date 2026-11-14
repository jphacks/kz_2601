package jp.kz2601.metamon.services.consent

import android.content.Context
import android.content.SharedPreferences

/**
 * 端末の小さな設定の置き場(iOS の UserDefaults に当たる)。同意・クラッシュ報告・AI ボタンの集計(#888)が使う。
 * テストとモックは [InMemoryKeyValueStore]。
 */
interface KeyValueStore {
    fun getInt(key: String): Int?

    fun getLong(key: String): Long?

    fun getBoolean(key: String): Boolean?

    fun getString(key: String): String?

    fun putInt(key: String, value: Int)

    fun putLong(key: String, value: Long)

    fun putBoolean(key: String, value: Boolean)

    fun putString(key: String, value: String)

    fun remove(key: String)
}

class InMemoryKeyValueStore : KeyValueStore {
    private val values = mutableMapOf<String, Any>()

    override fun getInt(key: String): Int? = values[key] as? Int

    override fun getLong(key: String): Long? = values[key] as? Long

    override fun getBoolean(key: String): Boolean? = values[key] as? Boolean

    override fun getString(key: String): String? = values[key] as? String

    override fun putInt(key: String, value: Int) = set(key, value)

    override fun putLong(key: String, value: Long) = set(key, value)

    override fun putBoolean(key: String, value: Boolean) = set(key, value)

    override fun putString(key: String, value: String) = set(key, value)

    override fun remove(key: String) {
        synchronized(values) { values.remove(key) }
    }

    private fun set(key: String, value: Any) {
        synchronized(values) { values[key] = value }
    }
}

/** SharedPreferences(端末だけ・バックアップの扱いはアプリの設定どおり)。 */
class SharedPreferencesKeyValueStore(private val prefs: SharedPreferences) : KeyValueStore {
    override fun getInt(key: String): Int? = if (prefs.contains(key)) prefs.getInt(key, 0) else null

    override fun getLong(key: String): Long? = if (prefs.contains(key)) prefs.getLong(key, 0) else null

    override fun getBoolean(key: String): Boolean? = if (prefs.contains(key)) prefs.getBoolean(key, false) else null

    override fun getString(key: String): String? = prefs.getString(key, null)

    override fun putInt(key: String, value: Int) = prefs.edit().putInt(key, value).apply()

    override fun putLong(key: String, value: Long) = prefs.edit().putLong(key, value).apply()

    override fun putBoolean(key: String, value: Boolean) = prefs.edit().putBoolean(key, value).apply()

    override fun putString(key: String, value: String) = prefs.edit().putString(key, value).apply()

    override fun remove(key: String) = prefs.edit().remove(key).apply()

    companion object {
        fun of(context: Context, name: String): SharedPreferencesKeyValueStore =
            SharedPreferencesKeyValueStore(context.applicationContext.getSharedPreferences(name, Context.MODE_PRIVATE))
    }
}
