package jp.kz2601.metamon.nearby

import android.content.Context
import android.os.Build
import android.provider.Settings
import jp.kz2601.metamon.runtime.UiText
import jp.kz2601.metamon.services.consent.KeyValueStore
import jp.kz2601.metamon.services.consent.SharedPreferencesKeyValueStore

/** 近距離共有の端末側の材料(通り道・設定・端末の名前・文言の解き方)。AppContainer が作る。 */
class NearbyEnv(
    val transport: () -> NearbyTransport,
    val settings: KeyValueStore,
    val deviceName: () -> String,
    val text: (UiText) -> String,
) {
    companion object {
        private const val PREFS = "metamon-nearby"

        /** [mock] なら端末の中だけの通り道(「受け取る」を押す相手が 1 人いる)。 */
        fun of(context: Context, mock: Boolean): NearbyEnv {
            val app = context.applicationContext
            return NearbyEnv(
                transport = { if (mock) MockNearbyTransport.withEchoPeer() else NearbyConnectionsTransport(app) },
                settings = SharedPreferencesKeyValueStore(app.getSharedPreferences(PREFS, Context.MODE_PRIVATE)),
                deviceName = { deviceName(app) },
                text = { it.resolve(app.resources) },
            )
        }

        /** 端末の名前(設定 › デバイス情報 の名前)。読めなければ機種名。 */
        fun deviceName(context: Context): String =
            Settings.Global.getString(context.contentResolver, Settings.Global.DEVICE_NAME)?.takeIf { it.isNotBlank() } ?: Build.MODEL
    }
}
