package jp.kz2601.metamon

import android.content.Context
import java.util.UUID

/** 端末の今のアプリの appId(iOS の `DeviceAppID`)。初回に UUID を発行して保存する。切替・リセットは android-061。 */
class DeviceAppId(context: Context) {
    private val prefs = context.applicationContext.getSharedPreferences("metamon", Context.MODE_PRIVATE)

    val current: String
        get() = prefs.getString(KEY, null)?.takeIf { it.isNotEmpty() }
            ?: UUID.randomUUID().toString().lowercase().also { prefs.edit().putString(KEY, it).apply() }

    private companion object {
        const val KEY = "metamon.appId"
    }
}
