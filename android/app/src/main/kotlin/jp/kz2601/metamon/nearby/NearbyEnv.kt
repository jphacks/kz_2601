package jp.kz2601.metamon.nearby

import android.content.Context
import android.os.Build
import android.provider.Settings
import com.google.android.gms.nearby.Nearby
import jp.kz2601.metamon.R
import jp.kz2601.metamon.runtime.UiText
import jp.kz2601.metamon.services.consent.KeyValueStore
import jp.kz2601.metamon.services.consent.SharedPreferencesKeyValueStore
import kotlinx.coroutines.CoroutineScope

/** 近距離共有の端末側の材料(通り道・設定・端末の名前・文言の解き方)。AppContainer が作る。 */
class NearbyEnv(
    val transport: () -> NearbyTransport,
    /** すれ違い通信(#1383)の通り道。モックは少し待って「テスト花子」とすれ違う */
    val passbyTransport: (CoroutineScope) -> PassbyTransport,
    val settings: KeyValueStore,
    val deviceName: () -> String,
    val text: (UiText) -> String,
    /** [settings] の置き場を全部消す(後片付け・#841) */
    val clearSettings: () -> Unit = {},
) {
    companion object {
        private const val PREFS = "metamon-nearby"

        /** [mock] なら端末の中だけの通り道(「受け取る」を押す相手が 1 人いる)。 */
        fun of(context: Context, mock: Boolean): NearbyEnv {
            val app = context.applicationContext
            // 2 つの機能で 1 つの client と止め方の持ち主を分け合う(片方が止めてももう片方を切らない)
            val client by lazy { Nearby.getConnectionsClient(app) }
            val shared by lazy { SharedNearbyClient.of(client) }
            return NearbyEnv(
                transport = { if (mock) MockNearbyTransport.withEchoPeer() else NearbyConnectionsTransport(client, shared) },
                passbyTransport = { scope ->
                    if (mock) {
                        MockPassbyTransport(scope)
                    } else {
                        NearbyConnectionsPassbyTransport(client, shared) { UiText.Res(R.string.nearby_discovery_failed).resolve(app.resources) }
                    }
                },
                settings = SharedPreferencesKeyValueStore(app.getSharedPreferences(PREFS, Context.MODE_PRIVATE)),
                clearSettings = { app.getSharedPreferences(PREFS, Context.MODE_PRIVATE).edit().clear().commit() },
                deviceName = { deviceName(app) },
                text = { it.resolve(app.resources) },
            )
        }

        /** 端末の名前(設定 › デバイス情報 の名前)。読めなければ機種名。 */
        fun deviceName(context: Context): String =
            Settings.Global.getString(context.contentResolver, Settings.Global.DEVICE_NAME)?.takeIf { it.isNotBlank() } ?: Build.MODEL
    }
}
