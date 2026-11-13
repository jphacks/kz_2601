package jp.kz2601.metamon.desktop.debug

import jp.kz2601.metamon.desktop.PaletteCommand
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.desktop.net.DesktopApi
import jp.kz2601.metamon.desktop.net.DesktopApiConfig

/** 開発用メニューの 1 項目。パレットに「開発用: <title>」で並ぶ。 */
class DevMenuItem(
    val id: String,
    val title: String,
    val keywords: List<String> = emptyList(),
    val run: () -> Unit,
)

/** 項目から触れるウィンドウの口(DesktopKeyboard が持つ)。 */
interface DevMenuHost {
    val config: DesktopApiConfig

    /** このアプリの送り待ちの件数。書き込み先が無ければ null。 */
    val pendingCount: Int?

    /** 「この端末のデータを消す」の口があるか。 */
    val canWipe: Boolean

    /** 文の並びを確認のダイアログで見せる。 */
    fun showInfo(title: String, lines: List<String>)

    /** 消す前の確認を出す(すぐには消さない)。 */
    fun askWipe()

    /** 下に短く知らせる。 */
    fun message(text: String) = Unit
}

/**
 * 開発用メニュー(#2089・iOS の #2079 と項目の名前を揃える)。
 * `TAILORIT_API=stg` の時だけ・配布物(jpackage)では出さない。既定の起動(通信しない)では出ない。
 */
object DevMenu {
    /** 項目を作るところ。足す時は 1 行ずつ・名前順。 */
    private val providers: List<(DevMenuHost) -> List<DevMenuItem>> = listOf(
        DevEnvironmentItems::items,
        DevPowerItems::items,
    )

    fun isEnabled(config: DesktopApiConfig, packaged: Boolean = isPackaged()): Boolean =
        config.mode == DesktopApiConfig.Mode.STG && !packaged

    /** jpackage の起動口から動いている(Autostart.kt と同じ見分け方)。 */
    fun isPackaged(appPath: String? = System.getProperty("jpackage.app-path")): Boolean = !appPath.isNullOrEmpty()

    fun items(host: DevMenuHost): List<DevMenuItem> = providers.flatMap { it(host) }

    /** パレットの行。出さない時は空。 */
    fun commands(host: DevMenuHost, enabled: Boolean = isEnabled(host.config)): List<PaletteCommand> {
        if (!enabled) return emptyList()
        return items(host).map { item ->
            PaletteCommand(
                "dev:${item.id}",
                Strings.format("desktop_dev_item", item.title),
                PaletteCommand.Kind.ACTION,
                keywords = item.keywords + listOf("dev", "debug", Strings["desktop_dev_menu"]),
                run = item.run,
            )
        }
    }
}

/** 「今の環境」と「この端末のデータを消す」(#3211)。 */
object DevEnvironmentItems {
    fun items(host: DevMenuHost): List<DevMenuItem> = listOfNotNull(
        DevMenuItem("environment", Strings["desktop_dev_environment"], listOf("environment", "env", "version")) {
            host.showInfo(Strings["desktop_dev_environment"], lines(host.config, host.pendingCount))
        },
        if (host.canWipe) DevMenuItem("wipe", Strings["desktop_wipe"], listOf("wipe", "reset")) { host.askWipe() } else null,
    )

    /** 接続先・版・送り待ち・トークンの有無(トークンそのものは出さない)。 */
    fun lines(
        config: DesktopApiConfig,
        pending: Int?,
        version: String = DesktopApi.VERSION,
        build: String = DesktopApi.BUILD,
    ): List<String> = listOf(
        Strings.format("desktop_dev_env_target", if (config.isLive) "${config.mode.name.lowercase()} (${config.appConfigUrl})" else Strings["desktop_dev_env_mock"]),
        Strings.format("desktop_dev_env_version", version, build),
        Strings.format("desktop_dev_env_pending", pending?.toString() ?: "-"),
        Strings.format("desktop_dev_env_token", if (config.devToken.isNullOrEmpty()) Strings["desktop_dev_env_token_none"] else Strings["desktop_dev_env_token_set"]),
    )
}
